/*
Copyright 2026 The Unified Platform Operator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	platformv1alpha1 "github.com/halildogan/upo/api/v1alpha1"
	"github.com/halildogan/upo/internal/metrics"
	"github.com/halildogan/upo/internal/platform"
	"github.com/halildogan/upo/internal/telemetry"
	"github.com/halildogan/upo/pkg/conditions"
)

const (
	tenantControllerName = "tenant"
	// tenantResyncPeriod bounds how long the controller will go without a level
	// re-check even in the absence of watch events, catching out-of-band drift.
	tenantResyncPeriod = 5 * time.Minute
)

// TenantReconciler reconciles a Tenant object into an isolated namespace with
// its ResourceQuota, LimitRange, default NetworkPolicy and admin RBAC.
type TenantReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=platform.upo.io,resources=tenants,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=platform.upo.io,resources=tenants/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=platform.upo.io,resources=tenants/finalizers,verbs=update
//+kubebuilder:rbac:groups=platform.upo.io,resources=environments;integrations,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=resourcequotas;limitranges,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is the level-based control loop for Tenant. It is invoked on every
// observed change to a Tenant (and to the resources it owns) and converges the
// cluster toward the Tenant's desired state. It is safe to call repeatedly.
func (r *TenantReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, retErr error) {
	ctx, span := telemetry.Tracer().Start(ctx, "TenantReconciler.Reconcile")
	defer span.End()

	start := time.Now()
	defer func() {
		outcome := metrics.ResultSuccess
		if retErr != nil {
			outcome = metrics.ResultError
		} else if result.RequeueAfter > 0 {
			outcome = metrics.ResultRequeue
		}
		metrics.RecordReconcile(tenantControllerName, outcome, time.Since(start).Seconds())
	}()

	tenant := &platformv1alpha1.Tenant{}
	if err := r.Get(ctx, req.NamespacedName, tenant); err != nil {
		if apierrors.IsNotFound(err) {
			metrics.ClearPhase(tenantControllerName, "", req.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle deletion via finalizer before anything else.
	if !tenant.DeletionTimestamp.IsZero() {
		return r.finalize(ctx, tenant)
	}

	// Ensure our finalizer is present so deletion is observable.
	if !controllerutil.ContainsFinalizer(tenant, platformv1alpha1.TenantFinalizer) {
		controllerutil.AddFinalizer(tenant, platformv1alpha1.TenantFinalizer)
		// The Update re-triggers reconciliation via the watch, so no explicit
		// requeue is needed (and Result.Requeue is deprecated).
		return ctrl.Result{}, r.Update(ctx, tenant)
	}

	return r.reconcileTenant(ctx, tenant)
}

// reconcileTenant performs the provisioning steps and records status. Each step
// is idempotent; a failure in any step records a Degraded condition, emits a
// Warning event, and returns the error so the work-queue retries with backoff.
func (r *TenantReconciler) reconcileTenant(ctx context.Context, tenant *platformv1alpha1.Tenant) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	gen := tenant.Generation
	suspended := tenant.Spec.Lifecycle.Suspended

	prefix := tenant.Spec.NamespacePrefix
	if prefix == "" {
		prefix = "tenant"
	}
	nsName := fmt.Sprintf("%s-%s", prefix, tenant.Name)
	tenant.Status.Namespace = nsName

	conditions.MarkTrue(&tenant.Status.Conditions, platformv1alpha1.ConditionProgressing,
		platformv1alpha1.ReasonReconciling, "Reconciling tenant resources", gen)

	labels := platform.TenantLabels(tenant.Name, "namespace", tenant.Spec.ExtraLabels)

	// 1. Namespace (cluster-scoped owner -> cluster-scoped dependent is allowed).
	if _, err := platform.ReconcileNamespace(ctx, r.Client, r.Scheme, tenant, true, nsName, labels); err != nil {
		return r.fail(ctx, tenant, "NamespaceError", err)
	}

	// 2. ResourceQuota (also enforces suspension by pinning pods to zero).
	var hard corev1.ResourceList
	if tenant.Spec.Quota != nil {
		hard = tenant.Spec.Quota.Hard
	}
	if _, err := platform.ReconcileResourceQuota(ctx, r.Client, r.Scheme, tenant, true, nsName, hard, suspended,
		platform.TenantLabels(tenant.Name, "quota", tenant.Spec.ExtraLabels)); err != nil {
		return r.fail(ctx, tenant, "QuotaError", err)
	}

	// 3. LimitRange (per-container defaults).
	var def, defReq, max corev1.ResourceList
	if tenant.Spec.Limits != nil {
		def, defReq, max = tenant.Spec.Limits.Default, tenant.Spec.Limits.DefaultRequest, tenant.Spec.Limits.Max
	}
	if _, err := platform.ReconcileLimitRange(ctx, r.Client, r.Scheme, tenant, true, nsName, def, defReq, max,
		platform.TenantLabels(tenant.Name, "limits", tenant.Spec.ExtraLabels)); err != nil {
		return r.fail(ctx, tenant, "LimitRangeError", err)
	}

	// 4. Default NetworkPolicy posture.
	if _, err := platform.ReconcileNetworkPolicy(ctx, r.Client, r.Scheme, tenant, nsName, tenant.Spec.NetworkIsolation,
		platform.TenantLabels(tenant.Name, "networkpolicy", tenant.Spec.ExtraLabels)); err != nil {
		return r.fail(ctx, tenant, "NetworkPolicyError", err)
	}

	// 5. Admin RBAC binding.
	if _, err := platform.ReconcileTenantAdminBinding(ctx, r.Client, r.Scheme, tenant, nsName, tenant.Spec.Admins,
		platform.TenantLabels(tenant.Name, "rbac", tenant.Spec.ExtraLabels)); err != nil {
		return r.fail(ctx, tenant, "RBACError", err)
	}

	// 6. Aggregate child counts for observability (efficient via field index).
	envCount, intgCount, err := r.countChildren(ctx, tenant.Name)
	if err != nil {
		return r.fail(ctx, tenant, "InventoryError", err)
	}
	tenant.Status.EnvironmentCount = envCount
	tenant.Status.IntegrationCount = intgCount

	// Success: clear Degraded, set Ready/phase, and emit a one-shot event on flip.
	conditions.Remove(&tenant.Status.Conditions, platformv1alpha1.ConditionDegraded)
	conditions.MarkFalse(&tenant.Status.Conditions, platformv1alpha1.ConditionProgressing,
		platformv1alpha1.ReasonProvisioned, "All tenant resources are provisioned", gen)

	if suspended {
		conditions.MarkFalse(&tenant.Status.Conditions, platformv1alpha1.ConditionReady,
			platformv1alpha1.ReasonSuspended, "Tenant is suspended; workloads are cordoned", gen)
		r.setPhase(tenant, platformv1alpha1.TenantPhaseSuspended)
	} else {
		wasReady := conditions.IsTrue(tenant.Status.Conditions, platformv1alpha1.ConditionReady)
		conditions.MarkTrue(&tenant.Status.Conditions, platformv1alpha1.ConditionReady,
			platformv1alpha1.ReasonProvisioned, "Tenant namespace and policies are active", gen)
		r.setPhase(tenant, platformv1alpha1.TenantPhaseActive)
		if !wasReady {
			r.Recorder.Eventf(tenant, corev1.EventTypeNormal, "Provisioned",
				"Tenant namespace %q provisioned and active", nsName)
		}
	}

	if err := r.updateStatus(ctx, tenant, gen); err != nil {
		return ctrl.Result{}, err
	}
	log.V(1).Info("tenant reconciled", "namespace", nsName, "environments", envCount, "integrations", intgCount)
	return ctrl.Result{RequeueAfter: tenantResyncPeriod}, nil
}

// finalize tears down provisioned resources honoring the DeletionPolicy, then
// removes the finalizer once teardown is complete.
func (r *TenantReconciler) finalize(ctx context.Context, tenant *platformv1alpha1.Tenant) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(tenant, platformv1alpha1.TenantFinalizer) {
		return ctrl.Result{}, nil
	}

	r.setPhase(tenant, platformv1alpha1.TenantPhaseTerminating)
	conditions.MarkFalse(&tenant.Status.Conditions, platformv1alpha1.ConditionReady,
		platformv1alpha1.ReasonTerminating, "Tenant is terminating", tenant.Generation)
	// Best-effort status update; ignore conflicts during teardown.
	_ = r.Status().Update(ctx, tenant)

	if tenant.Spec.Lifecycle.DeletionPolicy != platformv1alpha1.DeletionPolicyOrphan && tenant.Status.Namespace != "" {
		done, err := platform.EnsureNamespaceDeleted(ctx, r.Client, tenant.Status.Namespace)
		if err != nil {
			r.Recorder.Eventf(tenant, corev1.EventTypeWarning, "TeardownError",
				"Failed to delete namespace %q: %v", tenant.Status.Namespace, err)
			return ctrl.Result{}, err
		}
		if !done {
			// Namespace still terminating; requeue until it is gone.
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
	}

	metrics.ClearPhase(tenantControllerName, "", tenant.Name)
	controllerutil.RemoveFinalizer(tenant, platformv1alpha1.TenantFinalizer)
	if err := r.Update(ctx, tenant); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// countChildren returns the number of Environments and Integrations bound to the
// tenant, using the per-controller field indexes registered in SetupWithManager.
func (r *TenantReconciler) countChildren(ctx context.Context, tenantName string) (int32, int32, error) {
	var envs platformv1alpha1.EnvironmentList
	if err := r.List(ctx, &envs, client.MatchingFields{tenantRefIndexKey: tenantName}); err != nil {
		return 0, 0, err
	}
	var intgs platformv1alpha1.IntegrationList
	if err := r.List(ctx, &intgs, client.MatchingFields{tenantRefIndexKey: tenantName}); err != nil {
		return 0, 0, err
	}
	return int32(len(envs.Items)), int32(len(intgs.Items)), nil
}

// fail records a Degraded condition, emits a Warning event, persists status, and
// returns the error so the work queue retries with exponential backoff.
func (r *TenantReconciler) fail(ctx context.Context, tenant *platformv1alpha1.Tenant, reason string, cause error) (ctrl.Result, error) {
	conditions.MarkTrue(&tenant.Status.Conditions, platformv1alpha1.ConditionDegraded, reason, cause.Error(), tenant.Generation)
	conditions.MarkFalse(&tenant.Status.Conditions, platformv1alpha1.ConditionReady, reason, cause.Error(), tenant.Generation)
	r.setPhase(tenant, platformv1alpha1.TenantPhaseFailed)
	r.Recorder.Eventf(tenant, corev1.EventTypeWarning, reason, "%v", cause)
	if err := r.updateStatus(ctx, tenant, tenant.Generation); err != nil {
		// Prefer surfacing the original cause; log the status error.
		logf.FromContext(ctx).Error(err, "failed to update status while reporting failure")
	}
	return ctrl.Result{}, cause
}

// setPhase mutates the in-memory phase and updates the exported gauge.
func (r *TenantReconciler) setPhase(tenant *platformv1alpha1.Tenant, phase platformv1alpha1.TenantPhase) {
	tenant.Status.Phase = phase
	metrics.SetPhase(tenantControllerName, "", tenant.Name, string(phase))
}

// updateStatus stamps observedGeneration and timestamp, then writes the status
// subresource.
func (r *TenantReconciler) updateStatus(ctx context.Context, tenant *platformv1alpha1.Tenant, gen int64) error {
	tenant.Status.ObservedGeneration = gen
	now := metav1.Now()
	tenant.Status.LastReconcileTime = &now
	return r.Status().Update(ctx, tenant)
}

// SetupWithManager registers the controller, its owned resources, the child
// field indexes, and watches that map child changes back to their tenant.
func (r *TenantReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := indexTenantRef(mgr); err != nil {
		return err
	}
	mapToTenant := func(_ context.Context, obj client.Object) []reconcile.Request {
		var ref string
		switch o := obj.(type) {
		case *platformv1alpha1.Environment:
			ref = o.Spec.TenantRef.Name
		case *platformv1alpha1.Integration:
			ref = o.Spec.TenantRef.Name
		}
		if ref == "" {
			return nil
		}
		return []reconcile.Request{{NamespacedName: client.ObjectKey{Name: ref}}}
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1alpha1.Tenant{}).
		Owns(&corev1.Namespace{}).
		Owns(&corev1.ResourceQuota{}).
		Owns(&corev1.LimitRange{}).
		Watches(&platformv1alpha1.Environment{}, handler.EnqueueRequestsFromMapFunc(mapToTenant)).
		Watches(&platformv1alpha1.Integration{}, handler.EnqueueRequestsFromMapFunc(mapToTenant)).
		Complete(r)
}

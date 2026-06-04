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
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	platformv1alpha1 "github.com/halildogan/upo/api/v1alpha1"
	"github.com/halildogan/upo/internal/metrics"
	"github.com/halildogan/upo/internal/platform"
	"github.com/halildogan/upo/internal/telemetry"
	"github.com/halildogan/upo/pkg/conditions"
)

const (
	environmentControllerName = "environment"
	environmentResyncPeriod   = 3 * time.Minute
)

// EnvironmentReconciler reconciles an Environment object.
type EnvironmentReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=platform.upo.io,resources=environments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=platform.upo.io,resources=environments/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=platform.upo.io,resources=environments/finalizers,verbs=update
//+kubebuilder:rbac:groups=platform.upo.io,resources=tenants,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=resourcequotas;limitranges;configmaps;secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=argoproj.io,resources=applications,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=source.toolkit.fluxcd.io,resources=gitrepositories,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kustomize.toolkit.fluxcd.io,resources=kustomizations,verbs=get;list;watch;create;update;patch;delete

// Reconcile drives an Environment toward its desired state.
func (r *EnvironmentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, retErr error) {
	ctx, span := telemetry.Tracer().Start(ctx, "EnvironmentReconciler.Reconcile")
	defer span.End()

	start := time.Now()
	defer func() {
		outcome := metrics.ResultSuccess
		if retErr != nil {
			outcome = metrics.ResultError
		} else if result.RequeueAfter > 0 || result.Requeue {
			outcome = metrics.ResultRequeue
		}
		metrics.RecordReconcile(environmentControllerName, outcome, time.Since(start).Seconds())
	}()

	env := &platformv1alpha1.Environment{}
	if err := r.Get(ctx, req.NamespacedName, env); err != nil {
		if apierrors.IsNotFound(err) {
			metrics.ClearPhase(environmentControllerName, req.Namespace, req.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !env.DeletionTimestamp.IsZero() {
		return r.finalize(ctx, env)
	}

	if !controllerutil.ContainsFinalizer(env, platformv1alpha1.EnvironmentFinalizer) {
		controllerutil.AddFinalizer(env, platformv1alpha1.EnvironmentFinalizer)
		if err := r.Update(ctx, env); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	return r.reconcileEnvironment(ctx, env)
}

func (r *EnvironmentReconciler) reconcileEnvironment(ctx context.Context, env *platformv1alpha1.Environment) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	gen := env.Generation

	// Suspension short-circuits provisioning while leaving resources intact.
	if env.Spec.Suspend {
		conditions.MarkFalse(&env.Status.Conditions, platformv1alpha1.ConditionReady,
			platformv1alpha1.ReasonSuspended, "Environment is suspended", gen)
		r.setPhase(env, platformv1alpha1.EnvironmentPhaseSuspended)
		return ctrl.Result{}, r.updateStatus(ctx, env, gen)
	}

	// Resolve and validate the owning tenant.
	tenant := &platformv1alpha1.Tenant{}
	if err := r.Get(ctx, client.ObjectKey{Name: env.Spec.TenantRef.Name}, tenant); err != nil {
		if apierrors.IsNotFound(err) {
			return r.blockOnDependency(ctx, env, fmt.Sprintf("tenant %q not found", env.Spec.TenantRef.Name))
		}
		return ctrl.Result{}, err
	}
	if tenant.Status.Namespace == "" {
		return r.blockOnDependency(ctx, env, fmt.Sprintf("tenant %q namespace not yet provisioned", tenant.Name))
	}

	// Ephemeral TTL enforcement: compute and persist the expiry, delete on lapse.
	if env.Spec.Type == platformv1alpha1.EnvironmentTypeEphemeral && env.Spec.TTL != nil {
		expiry := env.CreationTimestamp.Add(env.Spec.TTL.Duration)
		expMeta := metav1.NewTime(expiry)
		env.Status.ExpiresAt = &expMeta
		if !time.Now().Before(expiry) {
			r.Recorder.Eventf(env, corev1.EventTypeNormal, "Expired",
				"Ephemeral environment reached its TTL (%s); deleting", env.Spec.TTL.Duration)
			if err := r.Delete(ctx, env); err != nil && !apierrors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	}

	conditions.MarkTrue(&env.Status.Conditions, platformv1alpha1.ConditionProgressing,
		platformv1alpha1.ReasonReconciling, "Reconciling environment resources", gen)

	targetNS := fmt.Sprintf("%s-%s", tenant.Status.Namespace, env.Name)
	env.Status.TargetNamespace = targetNS
	nsLabels := platform.MergeLabels(platform.TenantLabels(tenant.Name, "environment", nil), map[string]string{
		platform.EnvironmentLabel: env.Name,
	})

	// Workload namespace (cluster-scoped; owner is namespaced => no owner ref).
	if _, err := platform.ReconcileNamespace(ctx, r.Client, r.Scheme, env, false, targetNS, nsLabels); err != nil {
		return r.fail(ctx, env, "NamespaceError", err)
	}

	// Quota + limits derived from the profile (cross-namespace => no owner ref).
	if _, err := platform.ReconcileResourceQuota(ctx, r.Client, r.Scheme, env, false, targetNS,
		platform.ProfileQuota(env.Spec.Profile), false, nsLabels); err != nil {
		return r.fail(ctx, env, "QuotaError", err)
	}

	// Project non-secret configuration.
	if _, err := platform.ReconcileConfigMap(ctx, r.Client, r.Scheme, env, false, targetNS,
		platform.EnvConfigMapName, env.Spec.Variables, nsLabels); err != nil {
		return r.fail(ctx, env, "ConfigError", err)
	}

	// Replicate referenced secrets from the environment's namespace.
	if _, err := platform.ProjectSecrets(ctx, r.Client, r.Scheme, env, false, env.Namespace, targetNS,
		platform.EnvSecretName, env.Spec.SecretRefs, nsLabels); err != nil {
		return r.fail(ctx, env, "SecretError", err)
	}

	conditions.MarkTrue(&env.Status.Conditions, platformv1alpha1.ConditionReady,
		platformv1alpha1.ReasonProvisioned, "Workload namespace and configuration are provisioned", gen)

	// Delegate workload delivery to the GitOps engine, if configured.
	sync, err := platform.ReconcileGitOps(ctx, r.Client, r.Scheme, env, targetNS)
	if err != nil {
		return r.fail(ctx, env, "GitOpsError", err)
	}

	if env.Spec.Domain != "" {
		env.Status.URL = "https://" + env.Spec.Domain
	}

	r.derivePhase(env, sync, gen)
	conditions.MarkFalse(&env.Status.Conditions, platformv1alpha1.ConditionProgressing,
		platformv1alpha1.ReasonProvisioned, "Environment reconciled", gen)

	if err := r.updateStatus(ctx, env, gen); err != nil {
		return ctrl.Result{}, err
	}

	requeue := environmentResyncPeriod
	if env.Status.ExpiresAt != nil {
		if remaining := time.Until(env.Status.ExpiresAt.Time); remaining > 0 && remaining < requeue {
			requeue = remaining
		}
	}
	log.V(1).Info("environment reconciled", "targetNamespace", targetNS, "phase", env.Status.Phase)
	return ctrl.Result{RequeueAfter: requeue}, nil
}

// derivePhase maps the GitOps sync outcome onto the environment phase and the
// Sync status block.
func (r *EnvironmentReconciler) derivePhase(env *platformv1alpha1.Environment, sync *platformv1alpha1.SyncStatus, gen int64) {
	conditions.Remove(&env.Status.Conditions, platformv1alpha1.ConditionDegraded)
	if sync == nil {
		// No GitOps engine: healthy on the strength of namespace provisioning.
		env.Status.Sync = nil
		r.setPhase(env, platformv1alpha1.EnvironmentPhaseHealthy)
		return
	}
	now := metav1.Now()
	sync.LastSyncTime = &now
	env.Status.Sync = sync
	switch sync.Phase {
	case "Synced":
		r.setPhase(env, platformv1alpha1.EnvironmentPhaseHealthy)
	case "Error":
		conditions.MarkTrue(&env.Status.Conditions, platformv1alpha1.ConditionDegraded,
			"SyncError", sync.Message, gen)
		r.setPhase(env, platformv1alpha1.EnvironmentPhaseDegraded)
	default:
		r.setPhase(env, platformv1alpha1.EnvironmentPhaseSyncing)
	}
}

// blockOnDependency records that a prerequisite is missing and requeues.
func (r *EnvironmentReconciler) blockOnDependency(ctx context.Context, env *platformv1alpha1.Environment, msg string) (ctrl.Result, error) {
	conditions.MarkFalse(&env.Status.Conditions, platformv1alpha1.ConditionReady,
		platformv1alpha1.ReasonDependencyNotMet, msg, env.Generation)
	r.setPhase(env, platformv1alpha1.EnvironmentPhasePending)
	if err := r.updateStatus(ctx, env, env.Generation); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
}

func (r *EnvironmentReconciler) finalize(ctx context.Context, env *platformv1alpha1.Environment) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(env, platformv1alpha1.EnvironmentFinalizer) {
		return ctrl.Result{}, nil
	}
	r.setPhase(env, platformv1alpha1.EnvironmentPhaseTerminating)

	// Stop the GitOps engine first so it does not re-create torn-down resources.
	if err := platform.DeleteGitOps(ctx, r.Client, env); err != nil {
		return ctrl.Result{}, err
	}
	// Delete the workload namespace; its contents cascade.
	if env.Status.TargetNamespace != "" {
		done, err := platform.EnsureNamespaceDeleted(ctx, r.Client, env.Status.TargetNamespace)
		if err != nil {
			return ctrl.Result{}, err
		}
		if !done {
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
	}

	metrics.ClearPhase(environmentControllerName, env.Namespace, env.Name)
	controllerutil.RemoveFinalizer(env, platformv1alpha1.EnvironmentFinalizer)
	if err := r.Update(ctx, env); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *EnvironmentReconciler) fail(ctx context.Context, env *platformv1alpha1.Environment, reason string, cause error) (ctrl.Result, error) {
	conditions.MarkTrue(&env.Status.Conditions, platformv1alpha1.ConditionDegraded, reason, cause.Error(), env.Generation)
	conditions.MarkFalse(&env.Status.Conditions, platformv1alpha1.ConditionReady, reason, cause.Error(), env.Generation)
	r.setPhase(env, platformv1alpha1.EnvironmentPhaseFailed)
	r.Recorder.Eventf(env, corev1.EventTypeWarning, reason, "%v", cause)
	if err := r.updateStatus(ctx, env, env.Generation); err != nil {
		logf.FromContext(ctx).Error(err, "failed to update status while reporting failure")
	}
	return ctrl.Result{}, cause
}

func (r *EnvironmentReconciler) setPhase(env *platformv1alpha1.Environment, phase platformv1alpha1.EnvironmentPhase) {
	env.Status.Phase = phase
	metrics.SetPhase(environmentControllerName, env.Namespace, env.Name, string(phase))
}

func (r *EnvironmentReconciler) updateStatus(ctx context.Context, env *platformv1alpha1.Environment, gen int64) error {
	env.Status.ObservedGeneration = gen
	now := metav1.Now()
	env.Status.LastReconcileTime = &now
	return r.Status().Update(ctx, env)
}

// SetupWithManager wires the controller and the shared tenant-ref field index.
func (r *EnvironmentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := indexTenantRef(mgr); err != nil {
		return err
	}
	// The workload namespace and GitOps application live in different namespaces
	// than the Environment (or are cluster-scoped), so owner-reference-based
	// Owns() watches do not apply; periodic resync plus explicit requeues keep
	// status current.
	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1alpha1.Environment{}).
		Complete(r)
}

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
	integrationControllerName = "integration"
	integrationResyncPeriod   = 2 * time.Minute
	defaultInitialBackoff     = 5 * time.Second
	defaultMaxBackoff         = 5 * time.Minute
)

// IntegrationReconciler reconciles an Integration object. The Prober is
// injectable so the connection health check can be faked in tests; it defaults
// to a real HTTP prober when nil.
type IntegrationReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Prober   platform.Prober
}

//+kubebuilder:rbac:groups=platform.upo.io,resources=integrations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=platform.upo.io,resources=integrations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=platform.upo.io,resources=integrations/finalizers,verbs=update
//+kubebuilder:rbac:groups=platform.upo.io,resources=tenants,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile drives an Integration toward its desired state.
func (r *IntegrationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, retErr error) {
	ctx, span := telemetry.Tracer().Start(ctx, "IntegrationReconciler.Reconcile")
	defer span.End()

	start := time.Now()
	defer func() {
		outcome := metrics.ResultSuccess
		if retErr != nil {
			outcome = metrics.ResultError
		} else if result.RequeueAfter > 0 {
			outcome = metrics.ResultRequeue
		}
		metrics.RecordReconcile(integrationControllerName, outcome, time.Since(start).Seconds())
	}()

	intg := &platformv1alpha1.Integration{}
	if err := r.Get(ctx, req.NamespacedName, intg); err != nil {
		if apierrors.IsNotFound(err) {
			metrics.ClearPhase(integrationControllerName, req.Namespace, req.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !intg.DeletionTimestamp.IsZero() {
		return r.finalize(ctx, intg)
	}

	if !controllerutil.ContainsFinalizer(intg, platformv1alpha1.IntegrationFinalizer) {
		controllerutil.AddFinalizer(intg, platformv1alpha1.IntegrationFinalizer)
		// The Update re-triggers reconciliation via the watch (Result.Requeue is deprecated).
		return ctrl.Result{}, r.Update(ctx, intg)
	}

	return r.reconcileIntegration(ctx, intg)
}

func (r *IntegrationReconciler) reconcileIntegration(ctx context.Context, intg *platformv1alpha1.Integration) (ctrl.Result, error) {
	gen := intg.Generation
	intg.Status.ObservedEndpoint = intg.Spec.Endpoint

	if intg.Spec.Suspend {
		conditions.MarkFalse(&intg.Status.Conditions, platformv1alpha1.ConditionReady,
			platformv1alpha1.ReasonSuspended, "Integration is suspended", gen)
		r.setPhase(intg, platformv1alpha1.IntegrationPhaseDisabled)
		return ctrl.Result{}, r.updateStatus(ctx, intg, gen)
	}

	// Validate the owning tenant exists.
	tenant := &platformv1alpha1.Tenant{}
	if err := r.Get(ctx, client.ObjectKey{Name: intg.Spec.TenantRef.Name}, tenant); err != nil {
		if apierrors.IsNotFound(err) {
			return r.blockOnDependency(ctx, intg, fmt.Sprintf("tenant %q not found", intg.Spec.TenantRef.Name))
		}
		return ctrl.Result{}, err
	}

	conditions.MarkUnknown(&intg.Status.Conditions, platformv1alpha1.ConditionProgressing,
		platformv1alpha1.ReasonReconciling, "Resolving credentials and validating connectivity", gen)

	// Resolve credentials from the referenced Secret (same namespace).
	creds, err := platform.ResolveAuthSecret(ctx, r.Client, intg.Namespace, intg.Spec.AuthSecretRef)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return r.blockOnDependency(ctx, intg,
				fmt.Sprintf("auth secret %q not found", intg.Spec.AuthSecretRef.Name))
		}
		return r.fail(ctx, intg, "SecretError", err)
	}
	conditions.MarkTrue(&intg.Status.Conditions, "SecretResolved",
		platformv1alpha1.ReasonProvisioned, "Credentials resolved", gen)

	// Materialize the normalized connection secret consumed by workloads.
	connName := platform.ConnectionSecretName(intg.Name)
	data := platform.BuildConnectionData(intg.Spec, creds)
	labels := platform.TenantLabels(tenant.Name, "integration", nil)
	labels[platform.IntegrationLabel] = intg.Name
	if _, err := platform.ReconcileConnectionSecret(ctx, r.Client, r.Scheme, intg, intg.Namespace, connName, data, labels); err != nil {
		return r.fail(ctx, intg, "ConnectionSecretError", err)
	}
	intg.Status.ConnectionSecretName = connName

	// Health probe with retry/backoff.
	return r.probeAndFinish(ctx, intg, gen)
}

// probeAndFinish runs the health probe (when applicable) and translates the
// outcome into phase, conditions, retry counter, and requeue timing.
func (r *IntegrationReconciler) probeAndFinish(ctx context.Context, intg *platformv1alpha1.Integration, gen int64) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	now := metav1.Now()
	intg.Status.LastProbeTime = &now

	// Non-HTTP connector families cannot be probed by the HTTP prober; treat them
	// as connected on the strength of credential resolution and record why.
	if !platform.IsHTTPType(intg.Spec.Type) {
		return r.markConnected(ctx, intg, gen,
			fmt.Sprintf("connectivity assumed for type=%s (no HTTP probe)", intg.Spec.Type))
	}

	hc := intg.Spec.HealthCheck
	path, timeout := "/healthz", 10*time.Second
	if hc != nil {
		if hc.Path != "" {
			path = hc.Path
		}
		if hc.TimeoutSeconds > 0 {
			timeout = time.Duration(hc.TimeoutSeconds) * time.Second
		}
	}
	insecure := intg.Spec.TLS != nil && intg.Spec.TLS.InsecureSkipVerify

	err := r.prober().Probe(ctx, platform.ProbeRequest{
		Endpoint:           intg.Spec.Endpoint,
		HealthPath:         path,
		Timeout:            timeout,
		InsecureSkipVerify: insecure,
		Headers:            intg.Spec.Headers,
	})
	if err == nil {
		return r.markConnected(ctx, intg, gen, "endpoint reachable")
	}

	// Probe failed: apply retry/backoff semantics.
	intg.Status.Retries++
	intg.Status.LastError = err.Error()
	maxRetries := int32(5)
	if intg.Spec.RetryPolicy != nil {
		maxRetries = intg.Spec.RetryPolicy.MaxRetries
	}

	if intg.Status.Retries > maxRetries {
		conditions.MarkFalse(&intg.Status.Conditions, platformv1alpha1.ConditionReady,
			platformv1alpha1.ReasonProvisionFailed,
			fmt.Sprintf("health probe failed after %d retries: %v", maxRetries, err), gen)
		conditions.MarkTrue(&intg.Status.Conditions, platformv1alpha1.ConditionDegraded,
			"ProbeFailed", err.Error(), gen)
		r.setPhase(intg, platformv1alpha1.IntegrationPhaseFailed)
		r.Recorder.Eventf(intg, corev1.EventTypeWarning, "ProbeFailed",
			"Health probe exhausted %d retries: %v", maxRetries, err)
		if err := r.updateStatus(ctx, intg, gen); err != nil {
			return ctrl.Result{}, err
		}
		// Re-attempt at the resync cadence rather than tight backoff.
		return ctrl.Result{RequeueAfter: integrationResyncPeriod}, nil
	}

	backoff := computeBackoff(intg.Spec.RetryPolicy, intg.Status.Retries)
	conditions.MarkFalse(&intg.Status.Conditions, platformv1alpha1.ConditionReady,
		"Retrying", fmt.Sprintf("probe failed (attempt %d/%d), retrying in %s: %v",
			intg.Status.Retries, maxRetries, backoff, err), gen)
	r.setPhase(intg, platformv1alpha1.IntegrationPhaseDegraded)
	if err := r.updateStatus(ctx, intg, gen); err != nil {
		return ctrl.Result{}, err
	}
	log.V(1).Info("integration probe failed; backing off", "attempt", intg.Status.Retries, "backoff", backoff.String())
	return ctrl.Result{RequeueAfter: backoff}, nil
}

func (r *IntegrationReconciler) markConnected(ctx context.Context, intg *platformv1alpha1.Integration, gen int64, msg string) (ctrl.Result, error) {
	wasConnected := intg.Status.Phase == platformv1alpha1.IntegrationPhaseConnected
	intg.Status.Retries = 0
	intg.Status.LastError = ""
	conditions.Remove(&intg.Status.Conditions, platformv1alpha1.ConditionDegraded)
	conditions.MarkTrue(&intg.Status.Conditions, "Connected", platformv1alpha1.ReasonProvisioned, msg, gen)
	conditions.MarkTrue(&intg.Status.Conditions, platformv1alpha1.ConditionReady, platformv1alpha1.ReasonProvisioned, msg, gen)
	conditions.MarkFalse(&intg.Status.Conditions, platformv1alpha1.ConditionProgressing, platformv1alpha1.ReasonProvisioned, "Integration connected", gen)
	r.setPhase(intg, platformv1alpha1.IntegrationPhaseConnected)
	if !wasConnected {
		r.Recorder.Eventf(intg, corev1.EventTypeNormal, "Connected", "Integration %q connected: %s", intg.Name, msg)
	}
	return ctrl.Result{RequeueAfter: integrationResyncPeriod}, r.updateStatus(ctx, intg, gen)
}

func (r *IntegrationReconciler) blockOnDependency(ctx context.Context, intg *platformv1alpha1.Integration, msg string) (ctrl.Result, error) {
	conditions.MarkFalse(&intg.Status.Conditions, platformv1alpha1.ConditionReady,
		platformv1alpha1.ReasonDependencyNotMet, msg, intg.Generation)
	r.setPhase(intg, platformv1alpha1.IntegrationPhasePending)
	if err := r.updateStatus(ctx, intg, intg.Generation); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
}

func (r *IntegrationReconciler) finalize(ctx context.Context, intg *platformv1alpha1.Integration) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(intg, platformv1alpha1.IntegrationFinalizer) {
		return ctrl.Result{}, nil
	}
	// The connection secret is owner-referenced and will be GC'd, but delete it
	// explicitly so external credentials stop being mounted promptly.
	if intg.Status.ConnectionSecretName != "" {
		if err := platform.DeleteConnectionSecret(ctx, r.Client, intg.Namespace, intg.Status.ConnectionSecretName); err != nil {
			return ctrl.Result{}, err
		}
	}
	metrics.ClearPhase(integrationControllerName, intg.Namespace, intg.Name)
	controllerutil.RemoveFinalizer(intg, platformv1alpha1.IntegrationFinalizer)
	if err := r.Update(ctx, intg); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *IntegrationReconciler) fail(ctx context.Context, intg *platformv1alpha1.Integration, reason string, cause error) (ctrl.Result, error) {
	conditions.MarkTrue(&intg.Status.Conditions, platformv1alpha1.ConditionDegraded, reason, cause.Error(), intg.Generation)
	conditions.MarkFalse(&intg.Status.Conditions, platformv1alpha1.ConditionReady, reason, cause.Error(), intg.Generation)
	intg.Status.LastError = cause.Error()
	r.setPhase(intg, platformv1alpha1.IntegrationPhaseFailed)
	r.Recorder.Eventf(intg, corev1.EventTypeWarning, reason, "%v", cause)
	if err := r.updateStatus(ctx, intg, intg.Generation); err != nil {
		logf.FromContext(ctx).Error(err, "failed to update status while reporting failure")
	}
	return ctrl.Result{}, cause
}

func (r *IntegrationReconciler) setPhase(intg *platformv1alpha1.Integration, phase platformv1alpha1.IntegrationPhase) {
	intg.Status.Phase = phase
	metrics.SetPhase(integrationControllerName, intg.Namespace, intg.Name, string(phase))
}

func (r *IntegrationReconciler) updateStatus(ctx context.Context, intg *platformv1alpha1.Integration, gen int64) error {
	intg.Status.ObservedGeneration = gen
	now := metav1.Now()
	intg.Status.LastReconcileTime = &now
	return r.Status().Update(ctx, intg)
}

func (r *IntegrationReconciler) prober() platform.Prober {
	if r.Prober != nil {
		return r.Prober
	}
	return platform.HTTPProber{}
}

// computeBackoff returns the delay before the next retry for the given attempt
// (1-based), honoring the policy's strategy and bounds with sane defaults.
func computeBackoff(policy *platformv1alpha1.RetryPolicy, attempt int32) time.Duration {
	initial, max := defaultInitialBackoff, defaultMaxBackoff
	strategy := platformv1alpha1.BackoffStrategyExponential
	if policy != nil {
		if policy.InitialDelay != nil {
			initial = policy.InitialDelay.Duration
		}
		if policy.MaxDelay != nil {
			max = policy.MaxDelay.Duration
		}
		if policy.Backoff != "" {
			strategy = policy.Backoff
		}
	}
	if strategy == platformv1alpha1.BackoffStrategyFixed {
		return initial
	}
	// Exponential: initial * 2^(attempt-1), capped at max. Guard against overflow
	// by capping the shift.
	shift := attempt - 1
	if shift > 16 {
		shift = 16
	}
	d := initial * time.Duration(int64(1)<<uint(shift))
	if d <= 0 || d > max {
		return max
	}
	return d
}

// SetupWithManager wires the controller and the shared tenant-ref field index.
func (r *IntegrationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := indexTenantRef(mgr); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1alpha1.Integration{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}

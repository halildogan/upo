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

// Package metrics defines and registers the operator's custom Prometheus
// metrics with the controller-runtime metrics registry, so they are exposed on
// the same authenticated /metrics endpoint as the built-in controller metrics.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

const namespace = "upo"

var (
	// ReconcileTotal counts reconcile invocations by controller and outcome.
	ReconcileTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "reconcile_total",
		Help:      "Total number of reconcile invocations partitioned by controller and result (success|error|requeue).",
	}, []string{"controller", "result"})

	// ReconcileDuration observes reconcile wall-clock latency by controller.
	ReconcileDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "reconcile_duration_seconds",
		Help:      "Reconcile latency in seconds partitioned by controller.",
		Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	}, []string{"controller"})

	// ResourcePhase exposes the current coarse phase of each managed resource as
	// a one-hot gauge. Exactly one {phase} series per resource is set to 1; the
	// rest are deleted on transition to avoid stale series.
	ResourcePhase = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "resource_phase",
		Help:      "Current lifecycle phase of a managed resource (1 = active phase).",
	}, []string{"controller", "namespace", "name", "phase"})

	// TenantsManaged tracks the number of Tenant resources the operator manages.
	TenantsManaged = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "tenants_managed",
		Help:      "Number of Tenant resources currently observed by the operator.",
	})

	// IntegrationEventsDelivered counts events successfully delivered to externals.
	IntegrationEventsDelivered = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "integration_events_delivered_total",
		Help:      "Total events successfully delivered to an external system by an Integration.",
	}, []string{"namespace", "integration", "type"})

	// IntegrationEventsFailed counts events that exhausted their retry budget.
	IntegrationEventsFailed = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "integration_events_failed_total",
		Help:      "Total events that exhausted retries for an Integration.",
	}, []string{"namespace", "integration", "type"})
)

// Result label values for ReconcileTotal.
const (
	ResultSuccess = "success"
	ResultError   = "error"
	ResultRequeue = "requeue"
)

func init() {
	ctrlmetrics.Registry.MustRegister(
		ReconcileTotal,
		ReconcileDuration,
		ResourcePhase,
		TenantsManaged,
		IntegrationEventsDelivered,
		IntegrationEventsFailed,
	)
}

// RecordReconcile records the outcome and latency of a single reconcile.
func RecordReconcile(controller, result string, seconds float64) {
	ReconcileTotal.WithLabelValues(controller, result).Inc()
	ReconcileDuration.WithLabelValues(controller).Observe(seconds)
}

// SetPhase records the active phase for a resource as a one-hot gauge, deleting
// any previously-set phase series for the same resource first.
func SetPhase(controller, ns, name, phase string) {
	ResourcePhase.DeletePartialMatch(prometheus.Labels{
		"controller": controller,
		"namespace":  ns,
		"name":       name,
	})
	ResourcePhase.WithLabelValues(controller, ns, name, phase).Set(1)
}

// ClearPhase removes all phase series for a resource (call on deletion).
func ClearPhase(controller, ns, name string) {
	ResourcePhase.DeletePartialMatch(prometheus.Labels{
		"controller": controller,
		"namespace":  ns,
		"name":       name,
	})
}

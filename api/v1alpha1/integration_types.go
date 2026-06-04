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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IntegrationType selects the connector family the operator manages.
//
// +kubebuilder:validation:Enum=webhook;restapi;oauth2;database;messaging;objectstore
type IntegrationType string

const (
	IntegrationTypeWebhook     IntegrationType = "webhook"
	IntegrationTypeRESTAPI     IntegrationType = "restapi"
	IntegrationTypeOAuth2      IntegrationType = "oauth2"
	IntegrationTypeDatabase    IntegrationType = "database"
	IntegrationTypeMessaging   IntegrationType = "messaging"
	IntegrationTypeObjectStore IntegrationType = "objectstore"
)

// BackoffStrategy selects how retry delay grows between attempts.
//
// +kubebuilder:validation:Enum=fixed;exponential
type BackoffStrategy string

const (
	BackoffStrategyFixed       BackoffStrategy = "fixed"
	BackoffStrategyExponential BackoffStrategy = "exponential"
)

// IntegrationPhase is a coarse lifecycle phase surfaced as a printer column.
//
// +kubebuilder:validation:Enum=Pending;Connecting;Connected;Degraded;Failed;Disabled
type IntegrationPhase string

const (
	IntegrationPhasePending    IntegrationPhase = "Pending"
	IntegrationPhaseConnecting IntegrationPhase = "Connecting"
	IntegrationPhaseConnected  IntegrationPhase = "Connected"
	IntegrationPhaseDegraded   IntegrationPhase = "Degraded"
	IntegrationPhaseFailed     IntegrationPhase = "Failed"
	IntegrationPhaseDisabled   IntegrationPhase = "Disabled"
)

// RetryPolicy governs how the controller retries transient connector failures.
type RetryPolicy struct {
	// MaxRetries is the maximum number of consecutive retry attempts before the
	// integration is marked Failed and reconciliation backs off to the resync period.
	//
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=20
	// +kubebuilder:default=5
	// +optional
	MaxRetries int32 `json:"maxRetries,omitempty"`
	// Backoff selects fixed or exponential delay growth.
	//
	// +kubebuilder:default=exponential
	// +optional
	Backoff BackoffStrategy `json:"backoff,omitempty"`
	// InitialDelay is the delay before the first retry. Defaults to 5s.
	//
	// +optional
	InitialDelay *metav1.Duration `json:"initialDelay,omitempty"`
	// MaxDelay caps the per-attempt delay for exponential backoff. Defaults to 5m.
	//
	// +optional
	MaxDelay *metav1.Duration `json:"maxDelay,omitempty"`
}

// RateLimit declares client-side throttling applied to the connector.
type RateLimit struct {
	// RequestsPerSecond is the steady-state allowed request rate.
	//
	// +kubebuilder:validation:Minimum=1
	RequestsPerSecond int32 `json:"requestsPerSecond"`
	// Burst is the maximum momentary burst above the steady-state rate.
	//
	// +kubebuilder:validation:Minimum=1
	// +optional
	Burst int32 `json:"burst,omitempty"`
}

// HealthCheck configures active connectivity probing of the external endpoint.
type HealthCheck struct {
	// Path is appended to the endpoint for HTTP-style health probes.
	//
	// +kubebuilder:default=/healthz
	// +optional
	Path string `json:"path,omitempty"`
	// IntervalSeconds is how often to probe. Defaults to 60s.
	//
	// +kubebuilder:validation:Minimum=10
	// +kubebuilder:default=60
	// +optional
	IntervalSeconds int32 `json:"intervalSeconds,omitempty"`
	// TimeoutSeconds is the per-probe timeout. Defaults to 10s.
	//
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=10
	// +optional
	TimeoutSeconds int32 `json:"timeoutSeconds,omitempty"`
}

// TLSConfig tunes transport security for the connector.
type TLSConfig struct {
	// InsecureSkipVerify disables server certificate verification. Strongly
	// discouraged; intended only for self-signed endpoints in development.
	//
	// +optional
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
	// CASecretRef references a Secret containing a custom CA bundle (key "ca.crt").
	//
	// +optional
	CASecretRef *SecretReference `json:"caSecretRef,omitempty"`
}

// IntegrationSpec is the desired state of an Integration.
type IntegrationSpec struct {
	// TenantRef binds this integration to an owning, cluster-scoped Tenant. Immutable.
	TenantRef TenantReference `json:"tenantRef"`

	// Type selects the connector family.
	Type IntegrationType `json:"type"`

	// Endpoint is the base URL or DSN of the external system.
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2048
	Endpoint string `json:"endpoint"`

	// AuthSecretRef references the Secret holding credentials for the connector
	// (e.g. token, client id/secret, username/password). Same-namespace only.
	//
	// +optional
	AuthSecretRef *SecretReference `json:"authSecretRef,omitempty"`

	// Headers are static headers attached to outbound requests (webhook/restapi).
	//
	// +optional
	Headers map[string]string `json:"headers,omitempty"`

	// Events is the set of platform event types this integration subscribes to
	// (for webhook/messaging connectors), e.g. "tenant.created", "environment.synced".
	//
	// +optional
	// +listType=set
	Events []string `json:"events,omitempty"`

	// RetryPolicy governs transient-failure retries.
	//
	// +optional
	RetryPolicy *RetryPolicy `json:"retryPolicy,omitempty"`

	// RateLimit declares client-side throttling.
	//
	// +optional
	RateLimit *RateLimit `json:"rateLimit,omitempty"`

	// HealthCheck configures active connectivity probing.
	//
	// +optional
	HealthCheck *HealthCheck `json:"healthCheck,omitempty"`

	// TLS tunes transport security.
	//
	// +optional
	TLS *TLSConfig `json:"tls,omitempty"`

	// Suspend disables the integration without deleting it; the controller marks
	// it Disabled and stops probing/event delivery.
	//
	// +optional
	Suspend bool `json:"suspend,omitempty"`
}

// IntegrationStatus is the observed state of an Integration.
type IntegrationStatus struct {
	// Phase is a coarse lifecycle phase derived from Conditions.
	//
	// +optional
	Phase IntegrationPhase `json:"phase,omitempty"`

	// Conditions represent the latest available observations of the integration's state.
	//
	// +optional
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// ObservedGeneration is the .metadata.generation last reconciled.
	//
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// ConnectionSecretName is the name of the operator-managed Secret holding the
	// resolved, normalized connection configuration consumed by workloads.
	//
	// +optional
	ConnectionSecretName string `json:"connectionSecretName,omitempty"`

	// ObservedEndpoint echoes the endpoint last successfully validated.
	//
	// +optional
	ObservedEndpoint string `json:"observedEndpoint,omitempty"`

	// Retries is the number of consecutive failed connection attempts.
	//
	// +optional
	Retries int32 `json:"retries,omitempty"`

	// DeliveredEvents counts events successfully delivered to the external system.
	//
	// +optional
	DeliveredEvents int64 `json:"deliveredEvents,omitempty"`

	// FailedEvents counts events that exhausted retries.
	//
	// +optional
	FailedEvents int64 `json:"failedEvents,omitempty"`

	// LastEventTime records the most recent successful event delivery.
	//
	// +optional
	LastEventTime *metav1.Time `json:"lastEventTime,omitempty"`

	// LastProbeTime records the most recent health probe.
	//
	// +optional
	LastProbeTime *metav1.Time `json:"lastProbeTime,omitempty"`

	// LastError carries the most recent reconcile/probe error, if any.
	//
	// +optional
	LastError string `json:"lastError,omitempty"`

	// LastReconcileTime records when the controller last completed a reconcile.
	//
	// +optional
	LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=intg,categories=upo;platform
// +kubebuilder:printcolumn:name="Tenant",type=string,JSONPath=`.spec.tenantRef.name`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Delivered",type=integer,JSONPath=`.status.deliveredEvents`
// +kubebuilder:printcolumn:name="Failed",type=integer,JSONPath=`.status.failedEvents`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Integration models an external system connector (webhook, REST API, OAuth2,
// database, messaging, or object store) as a first-class, declarative resource
// with managed credentials, health probing, and retry semantics.
type Integration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IntegrationSpec   `json:"spec,omitempty"`
	Status IntegrationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// IntegrationList contains a list of Integration.
type IntegrationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Integration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Integration{}, &IntegrationList{})
}

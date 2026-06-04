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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TenantTier expresses the commercial/operational tier of a tenant. Controllers
// may use it to apply tier-aware defaults (e.g. priority classes, default quota).
//
// +kubebuilder:validation:Enum=free;standard;enterprise
type TenantTier string

const (
	TenantTierFree       TenantTier = "free"
	TenantTierStandard   TenantTier = "standard"
	TenantTierEnterprise TenantTier = "enterprise"
)

// NetworkIsolation selects the default NetworkPolicy posture applied to the
// tenant namespace.
//
// +kubebuilder:validation:Enum=None;Baseline;Strict
type NetworkIsolation string

const (
	// NetworkIsolationNone applies no NetworkPolicy (cluster default applies).
	NetworkIsolationNone NetworkIsolation = "None"
	// NetworkIsolationBaseline allows same-namespace traffic and DNS egress, denies the rest.
	NetworkIsolationBaseline NetworkIsolation = "Baseline"
	// NetworkIsolationStrict default-denies all ingress and egress; explicit policies must opt-in.
	NetworkIsolationStrict NetworkIsolation = "Strict"
)

// TenantPhase is a coarse, human-friendly lifecycle phase surfaced as a printer
// column. The authoritative machine state lives in Status.Conditions.
//
// +kubebuilder:validation:Enum=Pending;Provisioning;Active;Suspended;Terminating;Failed
type TenantPhase string

const (
	TenantPhasePending      TenantPhase = "Pending"
	TenantPhaseProvisioning TenantPhase = "Provisioning"
	TenantPhaseActive       TenantPhase = "Active"
	TenantPhaseSuspended    TenantPhase = "Suspended"
	TenantPhaseTerminating  TenantPhase = "Terminating"
	TenantPhaseFailed       TenantPhase = "Failed"
)

// TenantQuota maps directly onto a Kubernetes ResourceQuota for the tenant
// namespace. Using corev1.ResourceList keeps the contract identical to the
// upstream object operators and platform engineers already understand.
type TenantQuota struct {
	// Hard is the set of desired hard limits enforced for the tenant namespace.
	// Keys are standard ResourceQuota resource names, e.g. "requests.cpu",
	// "limits.memory", "pods", "services", "persistentvolumeclaims".
	//
	// +optional
	Hard corev1.ResourceList `json:"hard,omitempty"`
}

// TenantLimits maps onto a LimitRange applied to the tenant namespace, supplying
// per-container defaults so workloads that omit requests/limits remain bounded.
type TenantLimits struct {
	// Default is the default resource limit applied to containers that omit one.
	//
	// +optional
	Default corev1.ResourceList `json:"default,omitempty"`
	// DefaultRequest is the default resource request applied to containers that omit one.
	//
	// +optional
	DefaultRequest corev1.ResourceList `json:"defaultRequest,omitempty"`
	// Max is the maximum resource a single container may request.
	//
	// +optional
	Max corev1.ResourceList `json:"max,omitempty"`
}

// LifecyclePolicy controls suspension and deletion behaviour for a tenant.
type LifecyclePolicy struct {
	// Suspended, when true, scales the tenant down: workloads are cordoned via
	// an updated NetworkPolicy and a zero-pods ResourceQuota override, without
	// deleting any tenant data. Flip back to false to resume.
	//
	// +optional
	Suspended bool `json:"suspended,omitempty"`
	// DeletionPolicy decides whether the tenant namespace is deleted (default)
	// or orphaned when the Tenant resource is removed.
	//
	// +kubebuilder:default=Delete
	// +optional
	DeletionPolicy DeletionPolicy `json:"deletionPolicy,omitempty"`
}

// TenantSpec is the desired state of a Tenant.
type TenantSpec struct {
	// DisplayName is a human-friendly label for the tenant shown in UIs.
	//
	// +kubebuilder:validation:MaxLength=128
	// +optional
	DisplayName string `json:"displayName,omitempty"`

	// Tier selects the commercial/operational tier of the tenant.
	//
	// +kubebuilder:default=standard
	// +optional
	Tier TenantTier `json:"tier,omitempty"`

	// NamespacePrefix is prepended to the tenant name to form the provisioned
	// namespace (e.g. prefix "tenant" + name "acme" => "tenant-acme"). It is
	// immutable after creation; changing it would orphan the existing namespace.
	//
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	// +kubebuilder:validation:MaxLength=40
	// +kubebuilder:default=tenant
	// +optional
	NamespacePrefix string `json:"namespacePrefix,omitempty"`

	// Quota declares the hard resource limits enforced on the tenant namespace.
	//
	// +optional
	Quota *TenantQuota `json:"quota,omitempty"`

	// Limits declares per-container default requests/limits for the namespace.
	//
	// +optional
	Limits *TenantLimits `json:"limits,omitempty"`

	// NetworkIsolation selects the default NetworkPolicy posture.
	//
	// +kubebuilder:default=Baseline
	// +optional
	NetworkIsolation NetworkIsolation `json:"networkIsolation,omitempty"`

	// Admins are RBAC subjects granted the tenant-admin ClusterRole, bound within
	// the tenant namespace. They can manage workloads but not escape the tenant.
	//
	// +optional
	// +listType=atomic
	Admins []SubjectReference `json:"admins,omitempty"`

	// ExtraLabels are propagated onto the provisioned namespace and all
	// operator-managed child resources, useful for cost-allocation and policy.
	//
	// +optional
	ExtraLabels map[string]string `json:"extraLabels,omitempty"`

	// Lifecycle controls suspension and deletion behaviour.
	//
	// +optional
	Lifecycle LifecyclePolicy `json:"lifecycle,omitempty"`
}

// TenantStatus is the observed state of a Tenant.
type TenantStatus struct {
	// Phase is a coarse lifecycle phase derived from Conditions.
	//
	// +optional
	Phase TenantPhase `json:"phase,omitempty"`

	// Conditions represent the latest available observations of the tenant's state.
	//
	// +optional
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// ObservedGeneration is the .metadata.generation last reconciled by the controller.
	//
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Namespace is the name of the namespace provisioned for this tenant.
	//
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// EnvironmentCount is the number of Environment resources bound to this tenant.
	//
	// +optional
	EnvironmentCount int32 `json:"environmentCount,omitempty"`

	// IntegrationCount is the number of Integration resources bound to this tenant.
	//
	// +optional
	IntegrationCount int32 `json:"integrationCount,omitempty"`

	// LastReconcileTime records when the controller last completed a reconcile.
	//
	// +optional
	LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=tn,categories=upo;platform
// +kubebuilder:printcolumn:name="Tier",type=string,JSONPath=`.spec.tier`
// +kubebuilder:printcolumn:name="Namespace",type=string,JSONPath=`.status.namespace`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Tenant is the top-level multi-tenancy boundary of the platform. Reconciling a
// Tenant provisions an isolated namespace plus its ResourceQuota, LimitRange,
// default NetworkPolicy and RBAC bindings.
type Tenant struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TenantSpec   `json:"spec,omitempty"`
	Status TenantStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TenantList contains a list of Tenant.
type TenantList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Tenant `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Tenant{}, &TenantList{})
}

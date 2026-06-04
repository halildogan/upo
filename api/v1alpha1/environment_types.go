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

// EnvironmentType classifies the purpose of an environment. The type drives
// defaulting (e.g. ephemeral environments require a TTL) and policy decisions.
//
// +kubebuilder:validation:Enum=development;staging;production;preview;ephemeral
type EnvironmentType string

const (
	EnvironmentTypeDevelopment EnvironmentType = "development"
	EnvironmentTypeStaging     EnvironmentType = "staging"
	EnvironmentTypeProduction  EnvironmentType = "production"
	EnvironmentTypePreview     EnvironmentType = "preview"
	EnvironmentTypeEphemeral   EnvironmentType = "ephemeral"
)

// GitOpsProvider selects which GitOps engine the environment delegates sync to.
//
// +kubebuilder:validation:Enum=none;argocd;flux
type GitOpsProvider string

const (
	GitOpsProviderNone   GitOpsProvider = "none"
	GitOpsProviderArgoCD GitOpsProvider = "argocd"
	GitOpsProviderFlux   GitOpsProvider = "flux"
)

// EnvironmentPhase is a coarse lifecycle phase surfaced as a printer column.
//
// +kubebuilder:validation:Enum=Pending;Provisioning;Syncing;Healthy;Degraded;Suspended;Terminating;Failed
type EnvironmentPhase string

const (
	EnvironmentPhasePending      EnvironmentPhase = "Pending"
	EnvironmentPhaseProvisioning EnvironmentPhase = "Provisioning"
	EnvironmentPhaseSyncing      EnvironmentPhase = "Syncing"
	EnvironmentPhaseHealthy      EnvironmentPhase = "Healthy"
	EnvironmentPhaseDegraded     EnvironmentPhase = "Degraded"
	EnvironmentPhaseSuspended    EnvironmentPhase = "Suspended"
	EnvironmentPhaseTerminating  EnvironmentPhase = "Terminating"
	EnvironmentPhaseFailed       EnvironmentPhase = "Failed"
)

// GitSource declares the desired application source in a Git repository.
type GitSource struct {
	// RepoURL is the clone URL of the Git repository (https or ssh).
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^(https?://|git@|ssh://).+`
	RepoURL string `json:"repoURL"`
	// Revision is the branch, tag, or commit SHA to sync. Defaults to HEAD.
	//
	// +kubebuilder:default=HEAD
	// +optional
	Revision string `json:"revision,omitempty"`
	// Path is the directory within the repository containing manifests.
	//
	// +kubebuilder:default=.
	// +optional
	Path string `json:"path,omitempty"`
}

// GitOpsConfig configures delegation of reconciliation to a GitOps engine. When
// Provider is "none" the environment only provisions its namespace and leaves
// workload delivery to the platform team.
type GitOpsConfig struct {
	// Provider selects the GitOps engine.
	//
	// +kubebuilder:default=none
	Provider GitOpsProvider `json:"provider"`
	// AutoSync, when true, lets the GitOps engine apply changes automatically.
	//
	// +optional
	AutoSync bool `json:"autoSync,omitempty"`
	// Prune, when true, allows the GitOps engine to delete resources removed from source.
	//
	// +optional
	Prune bool `json:"prune,omitempty"`
}

// EnvironmentSpec is the desired state of an Environment.
type EnvironmentSpec struct {
	// TenantRef binds this environment to an owning, cluster-scoped Tenant. It is
	// immutable; rebinding an environment to a different tenant is not supported.
	TenantRef TenantReference `json:"tenantRef"`

	// Type classifies the environment and drives type-specific defaulting/policy.
	Type EnvironmentType `json:"type"`

	// Profile selects the compute footprint for the environment namespace.
	//
	// +kubebuilder:default=medium
	// +optional
	Profile ResourceProfile `json:"profile,omitempty"`

	// Source declares the Git source to deploy. Required when GitOps.Provider != none.
	//
	// +optional
	Source *GitSource `json:"source,omitempty"`

	// GitOps configures delegation to a GitOps engine.
	//
	// +optional
	GitOps GitOpsConfig `json:"gitops,omitempty"`

	// TTL is the lifetime of an ephemeral environment, after which the controller
	// deletes it. Required for type=ephemeral, ignored otherwise.
	//
	// +optional
	TTL *metav1.Duration `json:"ttl,omitempty"`

	// Domain is an optional ingress hostname surfaced in Status.URL once ready.
	//
	// +kubebuilder:validation:MaxLength=253
	// +optional
	Domain string `json:"domain,omitempty"`

	// Variables are non-secret configuration values projected into a ConfigMap
	// in the environment namespace for workloads to consume.
	//
	// +optional
	Variables map[string]string `json:"variables,omitempty"`

	// SecretRefs reference secret keys in the environment's namespace to be
	// surfaced to workloads. Cross-namespace references are disallowed.
	//
	// +optional
	// +listType=atomic
	SecretRefs []SecretKeySelector `json:"secretRefs,omitempty"`

	// Suspend pauses reconciliation and (for GitOps) sync, without deletion.
	//
	// +optional
	Suspend bool `json:"suspend,omitempty"`
}

// SyncStatus reports the most recent GitOps synchronization outcome.
type SyncStatus struct {
	// Revision is the resolved revision currently applied to the cluster.
	//
	// +optional
	Revision string `json:"revision,omitempty"`
	// Phase is the high-level sync state.
	//
	// +kubebuilder:validation:Enum=Unknown;Synced;OutOfSync;Error
	// +optional
	Phase string `json:"phase,omitempty"`
	// LastSyncTime is when the last successful sync completed.
	//
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`
	// Message carries human-readable detail about the sync state.
	//
	// +optional
	Message string `json:"message,omitempty"`
}

// EnvironmentStatus is the observed state of an Environment.
type EnvironmentStatus struct {
	// Phase is a coarse lifecycle phase derived from Conditions.
	//
	// +optional
	Phase EnvironmentPhase `json:"phase,omitempty"`

	// Conditions represent the latest available observations of the environment's state.
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

	// TargetNamespace is the workload namespace provisioned for this environment.
	//
	// +optional
	TargetNamespace string `json:"targetNamespace,omitempty"`

	// Sync reports the latest GitOps synchronization outcome.
	//
	// +optional
	Sync *SyncStatus `json:"sync,omitempty"`

	// URL is the externally reachable address of the environment, if any.
	//
	// +optional
	URL string `json:"url,omitempty"`

	// ExpiresAt is the computed deletion time for ephemeral environments.
	//
	// +optional
	ExpiresAt *metav1.Time `json:"expiresAt,omitempty"`

	// LastReconcileTime records when the controller last completed a reconcile.
	//
	// +optional
	LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=env,categories=upo;platform
// +kubebuilder:printcolumn:name="Tenant",type=string,JSONPath=`.spec.tenantRef.name`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.status.url`
// +kubebuilder:printcolumn:name="Expires",type=date,JSONPath=`.status.expiresAt`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Environment is a tenant-scoped, optionally GitOps-driven application
// environment. Reconciling an Environment provisions a workload namespace,
// projects configuration, and (when configured) drives a GitOps application.
type Environment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EnvironmentSpec   `json:"spec,omitempty"`
	Status EnvironmentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EnvironmentList contains a list of Environment.
type EnvironmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Environment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Environment{}, &EnvironmentList{})
}

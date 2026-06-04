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

// Finalizer names owned by the Unified Platform Operator. Each controller
// adds its finalizer on first reconcile and removes it only after the
// externally-managed resources it owns have been torn down, guaranteeing
// no orphaned namespaces, GitOps applications, or connector secrets.
const (
	// TenantFinalizer guards teardown of the tenant namespace and its scoped resources.
	TenantFinalizer = "platform.upo.io/tenant-protection"
	// EnvironmentFinalizer guards teardown of GitOps applications and workload namespaces.
	EnvironmentFinalizer = "platform.upo.io/environment-protection"
	// IntegrationFinalizer guards deregistration of external connectors and webhooks.
	IntegrationFinalizer = "platform.upo.io/integration-protection"
)

// Well-known condition types reported in the Status.Conditions of every UPO
// resource. They follow the Kubernetes API conventions (PascalCase, positive
// polarity) so they interoperate with `kubectl wait --for=condition=Ready`.
const (
	// ConditionReady is the top-level roll-up condition. True means the observed
	// state fully matches the desired spec at the observed generation.
	ConditionReady = "Ready"
	// ConditionProgressing is True while the controller is actively driving the
	// resource toward its desired state.
	ConditionProgressing = "Progressing"
	// ConditionDegraded is True when reconciliation is blocked or failing in a
	// way that requires attention (missing dependency, repeated errors).
	ConditionDegraded = "Degraded"
)

// Common, controller-agnostic condition reasons. Reasons are machine-readable
// CamelCase tokens; human detail belongs in the condition Message.
const (
	ReasonReconciling      = "Reconciling"
	ReasonProvisioned      = "Provisioned"
	ReasonProvisionFailed  = "ProvisionFailed"
	ReasonDependencyNotMet = "DependencyNotMet"
	ReasonSuspended        = "Suspended"
	ReasonTerminating      = "Terminating"
	ReasonValidationFailed = "ValidationFailed"
)

// DeletionPolicy controls what happens to externally-provisioned resources
// when the owning custom resource is deleted.
//
// +kubebuilder:validation:Enum=Delete;Orphan
type DeletionPolicy string

const (
	// DeletionPolicyDelete cascades deletion to all provisioned resources (default).
	DeletionPolicyDelete DeletionPolicy = "Delete"
	// DeletionPolicyOrphan leaves provisioned resources in place for manual cleanup.
	DeletionPolicyOrphan DeletionPolicy = "Orphan"
)

// ResourceProfile is a coarse t-shirt sizing for compute footprints. Controllers
// translate a profile into concrete requests/limits, keeping author-facing specs
// declarative and portable across clusters of different capacities.
//
// +kubebuilder:validation:Enum=small;medium;large;xlarge
type ResourceProfile string

const (
	ResourceProfileSmall  ResourceProfile = "small"
	ResourceProfileMedium ResourceProfile = "medium"
	ResourceProfileLarge  ResourceProfile = "large"
	ResourceProfileXLarge ResourceProfile = "xlarge"
)

// TenantReference identifies a cluster-scoped Tenant by name. It is used by the
// namespaced Environment and Integration resources to bind themselves to an
// owning tenant for authorization, labelling, and quota accounting.
type TenantReference struct {
	// Name is the metadata.name of the cluster-scoped Tenant resource.
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`
}

// SecretReference is a name-only reference to a Secret residing in the same
// namespace as the referencing resource. Cross-namespace secret references are
// intentionally disallowed to preserve tenant isolation boundaries.
type SecretReference struct {
	// Name of the Secret in the referencing resource's namespace.
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`
}

// SecretKeySelector references a single key within a namespaced Secret.
type SecretKeySelector struct {
	// Name of the Secret in the referencing resource's namespace.
	//
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// Key within the Secret's data map.
	//
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`
	// Optional, when true, suppresses errors if the Secret or key is absent.
	//
	// +optional
	Optional bool `json:"optional,omitempty"`
}

// SubjectReference identifies an RBAC subject to be granted access within a
// tenant. It mirrors the shape of rbacv1.Subject but is validated for the
// subset of kinds the operator supports.
type SubjectReference struct {
	// Kind of subject being referenced.
	//
	// +kubebuilder:validation:Enum=User;Group;ServiceAccount
	Kind string `json:"kind"`
	// Name of the subject.
	//
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// Namespace of the subject. Required only when Kind is ServiceAccount.
	//
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

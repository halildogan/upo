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

// Package platform contains the idempotent, side-effecting provisioning
// primitives shared by the UPO controllers: namespace, quota, limit range,
// RBAC, network policy, GitOps application, and external connector management.
//
// Every function in this package is designed to be safe to call on every
// reconcile: it computes desired state and converges the cluster toward it
// using server-side create-or-update semantics, never assuming a prior call.
package platform

const (
	// ManagedByLabel marks every resource the operator owns.
	ManagedByLabel = "app.kubernetes.io/managed-by"
	// ManagedByValue is the value written to ManagedByLabel.
	ManagedByValue = "unified-platform-operator"

	// TenantLabel records the owning tenant on provisioned resources.
	TenantLabel = "platform.upo.io/tenant"
	// EnvironmentLabel records the owning environment on provisioned resources.
	EnvironmentLabel = "platform.upo.io/environment"
	// IntegrationLabel records the owning integration on provisioned resources.
	IntegrationLabel = "platform.upo.io/integration"
	// ComponentLabel records the logical component (namespace, quota, rbac, ...).
	ComponentLabel = "platform.upo.io/component"
	// PartOfLabel ties resources to the platform for discovery.
	PartOfLabel = "app.kubernetes.io/part-of"
	// PartOfValue is the value written to PartOfLabel.
	PartOfValue = "upo-platform"
)

// BaseLabels returns the labels stamped onto every operator-managed resource.
func BaseLabels() map[string]string {
	return map[string]string{
		ManagedByLabel: ManagedByValue,
		PartOfLabel:    PartOfValue,
	}
}

// TenantLabels returns base labels plus tenant ownership and the component name,
// merged with any operator-author-supplied extra labels (which never override
// the reserved keys above).
func TenantLabels(tenant, component string, extra map[string]string) map[string]string {
	out := BaseLabels()
	for k, v := range extra {
		out[k] = v
	}
	out[TenantLabel] = tenant
	if component != "" {
		out[ComponentLabel] = component
	}
	return out
}

// MergeLabels returns a new map containing dst overlaid with src. Nil inputs are
// tolerated; reserved base labels in dst are preserved.
func MergeLabels(dst, src map[string]string) map[string]string {
	out := make(map[string]string, len(dst)+len(src))
	for k, v := range dst {
		out[k] = v
	}
	for k, v := range src {
		out[k] = v
	}
	return out
}

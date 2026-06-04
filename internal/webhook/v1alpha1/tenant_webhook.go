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

// Package v1alpha1 contains the admission webhooks (defaulting and validation)
// for the platform.upo.io/v1alpha1 custom resources.
package v1alpha1

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	platformv1alpha1 "github.com/halildogan/upo/api/v1alpha1"
)

// maxNamespaceNameLength is the RFC 1123 label limit that bounds the generated
// tenant namespace name (prefix + "-" + tenant name).
const maxNamespaceNameLength = 63

var tenantlog = logf.Log.WithName("tenant-webhook")

// tenantGroupKind identifies Tenant for structured validation errors.
var tenantGroupKind = schema.GroupKind{Group: platformv1alpha1.GroupVersion.Group, Kind: "Tenant"}

// SetupTenantWebhookWithManager registers the defaulting and validating webhooks.
func SetupTenantWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&platformv1alpha1.Tenant{}).
		WithDefaulter(&TenantCustomDefaulter{}).
		WithValidator(&TenantCustomValidator{}).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-platform-upo-io-v1alpha1-tenant,mutating=true,failurePolicy=fail,sideEffects=None,groups=platform.upo.io,resources=tenants,verbs=create;update,versions=v1alpha1,name=mtenant-v1alpha1.kb.io,admissionReviewVersions=v1

// TenantCustomDefaulter fills in defaults that are awkward to express purely via
// CRD defaults (e.g. cross-field defaults), keeping author-facing manifests terse.
type TenantCustomDefaulter struct{}

var _ webhook.CustomDefaulter = &TenantCustomDefaulter{}

// Default implements webhook.CustomDefaulter.
func (d *TenantCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	tenant, ok := obj.(*platformv1alpha1.Tenant)
	if !ok {
		return fmt.Errorf("expected a Tenant object but got %T", obj)
	}
	tenantlog.V(1).Info("defaulting tenant", "name", tenant.Name)

	if tenant.Spec.Tier == "" {
		tenant.Spec.Tier = platformv1alpha1.TenantTierStandard
	}
	if tenant.Spec.NamespacePrefix == "" {
		tenant.Spec.NamespacePrefix = "tenant"
	}
	if tenant.Spec.NetworkIsolation == "" {
		tenant.Spec.NetworkIsolation = platformv1alpha1.NetworkIsolationBaseline
	}
	if tenant.Spec.Lifecycle.DeletionPolicy == "" {
		tenant.Spec.Lifecycle.DeletionPolicy = platformv1alpha1.DeletionPolicyDelete
	}
	return nil
}

//+kubebuilder:webhook:path=/validate-platform-upo-io-v1alpha1-tenant,mutating=false,failurePolicy=fail,sideEffects=None,groups=platform.upo.io,resources=tenants,verbs=create;update,versions=v1alpha1,name=vtenant-v1alpha1.kb.io,admissionReviewVersions=v1

// TenantCustomValidator enforces invariants the CRD schema cannot express, such
// as the derived namespace length bound and immutability of the namespace prefix.
type TenantCustomValidator struct{}

var _ webhook.CustomValidator = &TenantCustomValidator{}

// ValidateCreate implements webhook.CustomValidator.
func (v *TenantCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	tenant, ok := obj.(*platformv1alpha1.Tenant)
	if !ok {
		return nil, fmt.Errorf("expected a Tenant object but got %T", obj)
	}
	return nil, v.validate(tenant)
}

// ValidateUpdate implements webhook.CustomValidator. The namespace prefix is
// immutable because changing it would orphan the already-provisioned namespace.
func (v *TenantCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldTenant, ok := oldObj.(*platformv1alpha1.Tenant)
	if !ok {
		return nil, fmt.Errorf("expected a Tenant object but got %T", oldObj)
	}
	newTenant, ok := newObj.(*platformv1alpha1.Tenant)
	if !ok {
		return nil, fmt.Errorf("expected a Tenant object but got %T", newObj)
	}
	var errs field.ErrorList
	if oldTenant.Spec.NamespacePrefix != newTenant.Spec.NamespacePrefix {
		errs = append(errs, field.Forbidden(
			field.NewPath("spec", "namespacePrefix"),
			"namespacePrefix is immutable once the tenant namespace is provisioned"))
	}
	if err := v.validateInto(newTenant, &errs); err != nil {
		return nil, err
	}
	if len(errs) > 0 {
		return nil, apierrors.NewInvalid(tenantGroupKind, newTenant.Name, errs)
	}
	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator. Deletion is always allowed;
// teardown safety is handled by the controller finalizer and DeletionPolicy.
func (v *TenantCustomValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (v *TenantCustomValidator) validate(tenant *platformv1alpha1.Tenant) error {
	var errs field.ErrorList
	if err := v.validateInto(tenant, &errs); err != nil {
		return err
	}
	if len(errs) > 0 {
		return apierrors.NewInvalid(tenantGroupKind, tenant.Name, errs)
	}
	return nil
}

func (v *TenantCustomValidator) validateInto(tenant *platformv1alpha1.Tenant, errs *field.ErrorList) error {
	prefix := tenant.Spec.NamespacePrefix
	if prefix == "" {
		prefix = "tenant"
	}
	nsName := fmt.Sprintf("%s-%s", prefix, tenant.Name)
	if len(nsName) > maxNamespaceNameLength {
		*errs = append(*errs, field.Invalid(
			field.NewPath("spec", "namespacePrefix"),
			prefix,
			fmt.Sprintf("derived namespace %q exceeds %d characters; shorten the prefix or tenant name", nsName, maxNamespaceNameLength)))
	}
	for i, a := range tenant.Spec.Admins {
		if a.Kind == "ServiceAccount" && a.Namespace == "" {
			*errs = append(*errs, field.Required(
				field.NewPath("spec", "admins").Index(i).Child("namespace"),
				"namespace is required for ServiceAccount subjects"))
		}
	}
	return nil
}

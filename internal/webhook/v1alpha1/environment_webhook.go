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

var environmentlog = logf.Log.WithName("environment-webhook")

var environmentGroupKind = schema.GroupKind{Group: platformv1alpha1.GroupVersion.Group, Kind: "Environment"}

// SetupEnvironmentWebhookWithManager registers the Environment webhooks.
func SetupEnvironmentWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&platformv1alpha1.Environment{}).
		WithDefaulter(&EnvironmentCustomDefaulter{}).
		WithValidator(&EnvironmentCustomValidator{}).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-platform-upo-io-v1alpha1-environment,mutating=true,failurePolicy=fail,sideEffects=None,groups=platform.upo.io,resources=environments,verbs=create;update,versions=v1alpha1,name=menvironment-v1alpha1.kb.io,admissionReviewVersions=v1

// EnvironmentCustomDefaulter applies defaults to Environment objects.
type EnvironmentCustomDefaulter struct{}

var _ webhook.CustomDefaulter = &EnvironmentCustomDefaulter{}

// Default implements webhook.CustomDefaulter.
func (d *EnvironmentCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	env, ok := obj.(*platformv1alpha1.Environment)
	if !ok {
		return fmt.Errorf("expected an Environment object but got %T", obj)
	}
	environmentlog.V(1).Info("defaulting environment", "name", env.Name, "namespace", env.Namespace)

	if env.Spec.Profile == "" {
		env.Spec.Profile = platformv1alpha1.ResourceProfileMedium
	}
	if env.Spec.GitOps.Provider == "" {
		env.Spec.GitOps.Provider = platformv1alpha1.GitOpsProviderNone
	}
	if env.Spec.Source != nil {
		if env.Spec.Source.Revision == "" {
			env.Spec.Source.Revision = "HEAD"
		}
		if env.Spec.Source.Path == "" {
			env.Spec.Source.Path = "."
		}
	}
	return nil
}

//+kubebuilder:webhook:path=/validate-platform-upo-io-v1alpha1-environment,mutating=false,failurePolicy=fail,sideEffects=None,groups=platform.upo.io,resources=environments,verbs=create;update,versions=v1alpha1,name=venvironment-v1alpha1.kb.io,admissionReviewVersions=v1

// EnvironmentCustomValidator enforces cross-field invariants and immutability.
type EnvironmentCustomValidator struct{}

var _ webhook.CustomValidator = &EnvironmentCustomValidator{}

// ValidateCreate implements webhook.CustomValidator.
func (v *EnvironmentCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	env, ok := obj.(*platformv1alpha1.Environment)
	if !ok {
		return nil, fmt.Errorf("expected an Environment object but got %T", obj)
	}
	var errs field.ErrorList
	v.validateInto(env, &errs)
	if len(errs) > 0 {
		return nil, apierrors.NewInvalid(environmentGroupKind, env.Name, errs)
	}
	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator. tenantRef and type are
// immutable: rebinding or repurposing an environment in place is unsafe.
func (v *EnvironmentCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldEnv, ok := oldObj.(*platformv1alpha1.Environment)
	if !ok {
		return nil, fmt.Errorf("expected an Environment object but got %T", oldObj)
	}
	newEnv, ok := newObj.(*platformv1alpha1.Environment)
	if !ok {
		return nil, fmt.Errorf("expected an Environment object but got %T", newObj)
	}
	var errs field.ErrorList
	if oldEnv.Spec.TenantRef.Name != newEnv.Spec.TenantRef.Name {
		errs = append(errs, field.Forbidden(field.NewPath("spec", "tenantRef"), "tenantRef is immutable"))
	}
	if oldEnv.Spec.Type != newEnv.Spec.Type {
		errs = append(errs, field.Forbidden(field.NewPath("spec", "type"), "type is immutable"))
	}
	v.validateInto(newEnv, &errs)
	if len(errs) > 0 {
		return nil, apierrors.NewInvalid(environmentGroupKind, newEnv.Name, errs)
	}
	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator.
func (v *EnvironmentCustomValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (v *EnvironmentCustomValidator) validateInto(env *platformv1alpha1.Environment, errs *field.ErrorList) {
	if env.Spec.TenantRef.Name == "" {
		*errs = append(*errs, field.Required(field.NewPath("spec", "tenantRef", "name"), "an owning tenant is required"))
	}
	if env.Spec.Type == platformv1alpha1.EnvironmentTypeEphemeral && env.Spec.TTL == nil {
		*errs = append(*errs, field.Required(field.NewPath("spec", "ttl"), "ttl is required for ephemeral environments"))
	}
	if env.Spec.GitOps.Provider != platformv1alpha1.GitOpsProviderNone {
		if env.Spec.Source == nil || env.Spec.Source.RepoURL == "" {
			*errs = append(*errs, field.Required(
				field.NewPath("spec", "source", "repoURL"),
				fmt.Sprintf("source.repoURL is required when gitops.provider=%s", env.Spec.GitOps.Provider)))
		}
	}
}

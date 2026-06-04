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
	"net/url"
	"strings"

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

var integrationlog = logf.Log.WithName("integration-webhook")

var integrationGroupKind = schema.GroupKind{Group: platformv1alpha1.GroupVersion.Group, Kind: "Integration"}

// SetupIntegrationWebhookWithManager registers the Integration webhooks.
func SetupIntegrationWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&platformv1alpha1.Integration{}).
		WithDefaulter(&IntegrationCustomDefaulter{}).
		WithValidator(&IntegrationCustomValidator{}).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-platform-upo-io-v1alpha1-integration,mutating=true,failurePolicy=fail,sideEffects=None,groups=platform.upo.io,resources=integrations,verbs=create;update,versions=v1alpha1,name=mintegration-v1alpha1.kb.io,admissionReviewVersions=v1

// IntegrationCustomDefaulter applies defaults to Integration objects.
type IntegrationCustomDefaulter struct{}

var _ webhook.CustomDefaulter = &IntegrationCustomDefaulter{}

// Default implements webhook.CustomDefaulter.
func (d *IntegrationCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	intg, ok := obj.(*platformv1alpha1.Integration)
	if !ok {
		return fmt.Errorf("expected an Integration object but got %T", obj)
	}
	integrationlog.V(1).Info("defaulting integration", "name", intg.Name, "namespace", intg.Namespace)

	if intg.Spec.RetryPolicy == nil {
		intg.Spec.RetryPolicy = &platformv1alpha1.RetryPolicy{
			MaxRetries: 5,
			Backoff:    platformv1alpha1.BackoffStrategyExponential,
		}
	} else {
		if intg.Spec.RetryPolicy.Backoff == "" {
			intg.Spec.RetryPolicy.Backoff = platformv1alpha1.BackoffStrategyExponential
		}
	}
	if platformv1alpha1.IntegrationType(intg.Spec.Type) != "" && intg.Spec.HealthCheck == nil {
		intg.Spec.HealthCheck = &platformv1alpha1.HealthCheck{
			Path:            "/healthz",
			IntervalSeconds: 60,
			TimeoutSeconds:  10,
		}
	}
	return nil
}

//+kubebuilder:webhook:path=/validate-platform-upo-io-v1alpha1-integration,mutating=false,failurePolicy=fail,sideEffects=None,groups=platform.upo.io,resources=integrations,verbs=create;update,versions=v1alpha1,name=vintegration-v1alpha1.kb.io,admissionReviewVersions=v1

// IntegrationCustomValidator enforces endpoint and immutability invariants.
type IntegrationCustomValidator struct{}

var _ webhook.CustomValidator = &IntegrationCustomValidator{}

// ValidateCreate implements webhook.CustomValidator.
func (v *IntegrationCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	intg, ok := obj.(*platformv1alpha1.Integration)
	if !ok {
		return nil, fmt.Errorf("expected an Integration object but got %T", obj)
	}
	var errs field.ErrorList
	v.validateInto(intg, &errs)
	if len(errs) > 0 {
		return nil, apierrors.NewInvalid(integrationGroupKind, intg.Name, errs)
	}
	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator. tenantRef and type are immutable.
func (v *IntegrationCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldIntg, ok := oldObj.(*platformv1alpha1.Integration)
	if !ok {
		return nil, fmt.Errorf("expected an Integration object but got %T", oldObj)
	}
	newIntg, ok := newObj.(*platformv1alpha1.Integration)
	if !ok {
		return nil, fmt.Errorf("expected an Integration object but got %T", newObj)
	}
	var errs field.ErrorList
	if oldIntg.Spec.TenantRef.Name != newIntg.Spec.TenantRef.Name {
		errs = append(errs, field.Forbidden(field.NewPath("spec", "tenantRef"), "tenantRef is immutable"))
	}
	if oldIntg.Spec.Type != newIntg.Spec.Type {
		errs = append(errs, field.Forbidden(field.NewPath("spec", "type"), "type is immutable"))
	}
	v.validateInto(newIntg, &errs)
	if len(errs) > 0 {
		return nil, apierrors.NewInvalid(integrationGroupKind, newIntg.Name, errs)
	}
	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator.
func (v *IntegrationCustomValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (v *IntegrationCustomValidator) validateInto(intg *platformv1alpha1.Integration, errs *field.ErrorList) {
	if intg.Spec.TenantRef.Name == "" {
		*errs = append(*errs, field.Required(field.NewPath("spec", "tenantRef", "name"), "an owning tenant is required"))
	}
	if strings.TrimSpace(intg.Spec.Endpoint) == "" {
		*errs = append(*errs, field.Required(field.NewPath("spec", "endpoint"), "endpoint is required"))
		return
	}
	// HTTP-family connectors must carry a parseable http(s) URL so the health
	// prober and workloads can rely on a well-formed endpoint.
	if isHTTPType(intg.Spec.Type) {
		u, err := url.Parse(intg.Spec.Endpoint)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			*errs = append(*errs, field.Invalid(
				field.NewPath("spec", "endpoint"),
				intg.Spec.Endpoint,
				fmt.Sprintf("endpoint must be a valid http(s) URL for type=%s", intg.Spec.Type)))
		}
	}
}

// isHTTPType mirrors platform.IsHTTPType without importing the platform package
// into the webhook layer, keeping the admission path dependency-light.
func isHTTPType(t platformv1alpha1.IntegrationType) bool {
	switch t {
	case platformv1alpha1.IntegrationTypeWebhook,
		platformv1alpha1.IntegrationTypeRESTAPI,
		platformv1alpha1.IntegrationTypeOAuth2,
		platformv1alpha1.IntegrationTypeObjectStore:
		return true
	default:
		return false
	}
}

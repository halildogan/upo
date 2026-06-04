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
	"testing"

	platformv1alpha1 "github.com/halildogan/upo/api/v1alpha1"
)

// TestIntegrationValidateCreate is a pure unit test (no cluster required) for the
// admission validation rules.
func TestIntegrationValidateCreate(t *testing.T) {
	v := &IntegrationCustomValidator{}

	tests := []struct {
		name    string
		spec    platformv1alpha1.IntegrationSpec
		wantErr bool
	}{
		{
			name: "valid https webhook",
			spec: platformv1alpha1.IntegrationSpec{
				TenantRef: platformv1alpha1.TenantReference{Name: "acme"},
				Type:      platformv1alpha1.IntegrationTypeWebhook,
				Endpoint:  "https://hooks.example.com/ingest",
			},
		},
		{
			name: "missing tenantRef",
			spec: platformv1alpha1.IntegrationSpec{
				Type:     platformv1alpha1.IntegrationTypeWebhook,
				Endpoint: "https://hooks.example.com",
			},
			wantErr: true,
		},
		{
			name: "http type with non-url endpoint",
			spec: platformv1alpha1.IntegrationSpec{
				TenantRef: platformv1alpha1.TenantReference{Name: "acme"},
				Type:      platformv1alpha1.IntegrationTypeRESTAPI,
				Endpoint:  "not a url",
			},
			wantErr: true,
		},
		{
			name: "database type allows DSN endpoint",
			spec: platformv1alpha1.IntegrationSpec{
				TenantRef: platformv1alpha1.TenantReference{Name: "acme"},
				Type:      platformv1alpha1.IntegrationTypeDatabase,
				Endpoint:  "postgres://db.internal:5432/app",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intg := &platformv1alpha1.Integration{Spec: tt.spec}
			_, err := v.ValidateCreate(context.Background(), intg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateCreate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestIntegrationImmutability verifies tenantRef and type cannot change.
func TestIntegrationImmutability(t *testing.T) {
	v := &IntegrationCustomValidator{}
	old := &platformv1alpha1.Integration{Spec: platformv1alpha1.IntegrationSpec{
		TenantRef: platformv1alpha1.TenantReference{Name: "acme"},
		Type:      platformv1alpha1.IntegrationTypeWebhook,
		Endpoint:  "https://a.example.com",
	}}
	updated := old.DeepCopy()
	updated.Spec.TenantRef.Name = "other"

	if _, err := v.ValidateUpdate(context.Background(), old, updated); err == nil {
		t.Fatalf("expected error when mutating immutable tenantRef")
	}
}

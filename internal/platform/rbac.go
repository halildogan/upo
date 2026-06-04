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

package platform

import (
	"context"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	platformv1alpha1 "github.com/halildogan/upo/api/v1alpha1"
)

// TenantAdminBindingName is the deterministic name of the RoleBinding that
// grants tenant admins the built-in "admin" ClusterRole within the tenant
// namespace. Binding to the aggregated "admin" role lets tenant admins manage
// workloads while remaining fully confined to their namespace.
const TenantAdminBindingName = "upo-tenant-admins"

// ReconcileTenantAdminBinding converges a namespaced RoleBinding granting the
// configured admin subjects the "admin" ClusterRole. When no admins are
// configured the managed binding is removed.
func ReconcileTenantAdminBinding(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	owner client.Object,
	namespace string,
	admins []platformv1alpha1.SubjectReference,
	labels map[string]string,
) (controllerutil.OperationResult, error) {
	rb := &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: TenantAdminBindingName, Namespace: namespace}}

	if len(admins) == 0 {
		if err := c.Delete(ctx, rb); err != nil && !ignoreNotFound(err) {
			return controllerutil.OperationResultNone, err
		}
		return controllerutil.OperationResultNone, nil
	}

	return controllerutil.CreateOrUpdate(ctx, c, rb, func() error {
		rb.Labels = MergeLabels(rb.Labels, labels)
		// RoleRef is immutable once set; CreateOrUpdate writes it on create and
		// leaves it untouched on update, which matches the API's constraint.
		rb.RoleRef = rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     "admin",
		}
		rb.Subjects = toSubjects(admins)
		return controllerutil.SetControllerReference(owner, rb, scheme)
	})
}

// toSubjects maps the operator's validated SubjectReference list onto rbac
// Subjects, filling in the canonical API group for each subject kind.
func toSubjects(admins []platformv1alpha1.SubjectReference) []rbacv1.Subject {
	subjects := make([]rbacv1.Subject, 0, len(admins))
	for _, a := range admins {
		s := rbacv1.Subject{Kind: a.Kind, Name: a.Name}
		switch a.Kind {
		case rbacv1.ServiceAccountKind:
			s.Namespace = a.Namespace
			s.APIGroup = ""
		default: // User, Group
			s.APIGroup = rbacv1.GroupName
		}
		subjects = append(subjects, s)
	}
	return subjects
}

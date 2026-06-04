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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// ReconcileNamespace converges a namespace to the desired labels, optionally
// setting a controller owner reference. setController must be false when the
// owner is namespace-scoped (a cluster-scoped Namespace cannot have a
// namespaced owner); such namespaces are torn down explicitly via finalizers.
//
// It is idempotent and returns the operation performed for logging/metrics.
func ReconcileNamespace(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	owner client.Object,
	setController bool,
	name string,
	labels map[string]string,
) (controllerutil.OperationResult, error) {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	return controllerutil.CreateOrUpdate(ctx, c, ns, func() error {
		if ns.Labels == nil {
			ns.Labels = map[string]string{}
		}
		for k, v := range labels {
			ns.Labels[k] = v
		}
		if setController {
			return controllerutil.SetControllerReference(owner, ns, scheme)
		}
		return nil
	})
}

// EnsureNamespaceDeleted deletes the named namespace if it exists and is not
// already terminating. It is safe to call repeatedly from a finalizer: a
// NotFound is treated as success, and an in-progress deletion is a no-op.
//
// It returns done=true once the namespace is fully gone so the caller can drop
// its finalizer.
func EnsureNamespaceDeleted(ctx context.Context, c client.Client, name string) (done bool, err error) {
	ns := &corev1.Namespace{}
	if err := c.Get(ctx, client.ObjectKey{Name: name}, ns); err != nil {
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
	if ns.DeletionTimestamp.IsZero() {
		if err := c.Delete(ctx, ns); err != nil && !apierrors.IsNotFound(err) {
			return false, err
		}
	}
	// Still present (terminating); caller should requeue until it disappears.
	return false, nil
}

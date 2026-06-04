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

package controller

import (
	"context"
	"sync"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	platformv1alpha1 "github.com/halildogan/upo/api/v1alpha1"
)

// tenantRefIndexKey is the cache field index that maps Environment and
// Integration resources to the name of the Tenant they reference. It enables
// O(1) "list children of a tenant" queries instead of full namespace scans, and
// it is shared by all three controllers.
const tenantRefIndexKey = ".spec.tenantRef.name"

// indexOnce guarantees the field indexes are registered exactly once even if
// multiple controllers call the registration helper during manager setup.
var indexOnce sync.Once
var indexErr error

// indexTenantRef registers the spec.tenantRef.name index for both child kinds.
// Registering an index more than once panics in controller-runtime, so the
// work is guarded by a sync.Once and the cached error is returned to every
// caller.
func indexTenantRef(mgr ctrl.Manager) error {
	indexOnce.Do(func() {
		ctx := context.Background()
		if err := mgr.GetFieldIndexer().IndexField(ctx, &platformv1alpha1.Environment{}, tenantRefIndexKey,
			func(o client.Object) []string {
				return []string{o.(*platformv1alpha1.Environment).Spec.TenantRef.Name}
			}); err != nil {
			indexErr = err
			return
		}
		if err := mgr.GetFieldIndexer().IndexField(ctx, &platformv1alpha1.Integration{}, tenantRefIndexKey,
			func(o client.Object) []string {
				return []string{o.(*platformv1alpha1.Integration).Spec.TenantRef.Name}
			}); err != nil {
			indexErr = err
			return
		}
	})
	return indexErr
}

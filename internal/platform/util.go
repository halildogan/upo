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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// mustQuantity parses a resource quantity string that is known at compile time
// to be valid (operator-internal constants only). It panics on malformed input
// because such a panic indicates a programming error, not a runtime condition.
func mustQuantity(s string) resource.Quantity {
	return resource.MustParse(s)
}

// ignoreNotFound returns true if err is a Kubernetes NotFound error, allowing
// callers to treat "already gone" as success during teardown.
func ignoreNotFound(err error) bool {
	return apierrors.IsNotFound(err)
}

// applyOwner conditionally sets a controller owner reference. It is a no-op when
// setController is false, which callers use for resources living in a different
// namespace than the owner (cross-namespace owner references are invalid).
func applyOwner(setController bool, owner client.Object, controlled client.Object, scheme *runtime.Scheme) error {
	if !setController {
		return nil
	}
	return controllerutil.SetControllerReference(owner, controlled, scheme)
}

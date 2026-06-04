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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	platformv1alpha1 "github.com/halildogan/upo/api/v1alpha1"
)

// EnvConfigMapName is the deterministic name of the projected env config map.
const EnvConfigMapName = "upo-env-config"

// EnvSecretName is the deterministic name of the projected env secret.
const EnvSecretName = "upo-env-secrets"

// ReconcileConfigMap converges a ConfigMap of plain string data. setController
// must be false when the ConfigMap is in a different namespace than the owner.
func ReconcileConfigMap(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	owner client.Object,
	setController bool,
	namespace, name string,
	data map[string]string,
	labels map[string]string,
) (controllerutil.OperationResult, error) {
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
	if len(data) == 0 {
		if err := c.Delete(ctx, cm); err != nil && !apierrors.IsNotFound(err) {
			return controllerutil.OperationResultNone, err
		}
		return controllerutil.OperationResultNone, nil
	}
	return controllerutil.CreateOrUpdate(ctx, c, cm, func() error {
		cm.Labels = MergeLabels(cm.Labels, labels)
		cm.Data = data
		return applyOwner(setController, owner, cm, scheme)
	})
}

// ProjectSecrets resolves a list of SecretKeySelectors from the source namespace
// and replicates the selected keys into a single Secret in the destination
// namespace. Optional selectors that are absent are skipped; a required selector
// that is missing returns an error so the caller can surface DependencyNotMet.
//
// This is the cross-namespace secret replication an environment uses to surface
// tenant-namespace credentials to its (separate) workload namespace, where
// direct cross-namespace references are not permitted.
func ProjectSecrets(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	owner client.Object,
	setController bool,
	srcNamespace, dstNamespace, name string,
	selectors []platformv1alpha1.SecretKeySelector,
	labels map[string]string,
) (controllerutil.OperationResult, error) {
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: dstNamespace}}

	if len(selectors) == 0 {
		if err := c.Delete(ctx, secret); err != nil && !apierrors.IsNotFound(err) {
			return controllerutil.OperationResultNone, err
		}
		return controllerutil.OperationResultNone, nil
	}

	collected := map[string][]byte{}
	for _, sel := range selectors {
		src := &corev1.Secret{}
		if err := c.Get(ctx, client.ObjectKey{Namespace: srcNamespace, Name: sel.Name}, src); err != nil {
			if apierrors.IsNotFound(err) && sel.Optional {
				continue
			}
			return controllerutil.OperationResultNone, fmt.Errorf("resolve secret %s/%s: %w", srcNamespace, sel.Name, err)
		}
		val, ok := src.Data[sel.Key]
		if !ok {
			if sel.Optional {
				continue
			}
			return controllerutil.OperationResultNone, fmt.Errorf("key %q absent from secret %s/%s", sel.Key, srcNamespace, sel.Name)
		}
		collected[sel.Key] = val
	}

	return controllerutil.CreateOrUpdate(ctx, c, secret, func() error {
		secret.Labels = MergeLabels(secret.Labels, labels)
		secret.Type = corev1.SecretTypeOpaque
		secret.Data = collected
		return applyOwner(setController, owner, secret, scheme)
	})
}

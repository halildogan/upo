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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// QuotaName is the deterministic name of the operator-managed ResourceQuota.
const QuotaName = "upo-tenant-quota"

// LimitRangeName is the deterministic name of the operator-managed LimitRange.
const LimitRangeName = "upo-tenant-limits"

// ReconcileResourceQuota converges a single ResourceQuota in the tenant
// namespace to the desired hard limits. When suspended is true, the quota is
// overridden to pin the namespace to zero pods, cordoning all workloads without
// deleting any data — the implementation of tenant suspension.
//
// setController controls whether a controller owner reference is set. It must be
// false when the quota lives in a different namespace than the owner (e.g. an
// Environment provisioning a separate workload namespace), since cross-namespace
// owner references are invalid; such resources are cleaned up by namespace cascade.
func ReconcileResourceQuota(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	owner client.Object,
	setController bool,
	namespace string,
	hard corev1.ResourceList,
	suspended bool,
	labels map[string]string,
) (controllerutil.OperationResult, error) {
	rq := &corev1.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: QuotaName, Namespace: namespace}}

	// When no hard limits are declared and the tenant is not suspended, there is
	// nothing to enforce: remove any previously-managed quota rather than leaving
	// an empty (and therefore unbounded) ResourceQuota in place.
	if len(hard) == 0 && !suspended {
		if err := c.Delete(ctx, rq); err != nil && !ignoreNotFound(err) {
			return controllerutil.OperationResultNone, err
		}
		return controllerutil.OperationResultNone, nil
	}

	return controllerutil.CreateOrUpdate(ctx, c, rq, func() error {
		rq.Labels = MergeLabels(rq.Labels, labels)
		desired := corev1.ResourceList{}
		for k, v := range hard {
			desired[k] = v
		}
		if suspended {
			// Hard-cap pods at zero; existing pods are not evicted but no new
			// pods may be scheduled, achieving a reversible "suspend".
			desired[corev1.ResourcePods] = mustQuantity("0")
		}
		rq.Spec.Hard = desired
		return applyOwner(setController, owner, rq, scheme)
	})
}

// ReconcileLimitRange converges the per-container default request/limit policy
// for the tenant namespace. When all three maps are empty the managed
// LimitRange is removed so it does not impose an unintended policy.
func ReconcileLimitRange(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	owner client.Object,
	setController bool,
	namespace string,
	def, defReq, max corev1.ResourceList,
	labels map[string]string,
) (controllerutil.OperationResult, error) {
	lr := &corev1.LimitRange{ObjectMeta: metav1.ObjectMeta{Name: LimitRangeName, Namespace: namespace}}

	if len(def) == 0 && len(defReq) == 0 && len(max) == 0 {
		if err := c.Delete(ctx, lr); err != nil && !ignoreNotFound(err) {
			return controllerutil.OperationResultNone, err
		}
		return controllerutil.OperationResultNone, nil
	}

	return controllerutil.CreateOrUpdate(ctx, c, lr, func() error {
		lr.Labels = MergeLabels(lr.Labels, labels)
		lr.Spec.Limits = []corev1.LimitRangeItem{{
			Type:           corev1.LimitTypeContainer,
			Default:        nonEmpty(def),
			DefaultRequest: nonEmpty(defReq),
			Max:            nonEmpty(max),
		}}
		return applyOwner(setController, owner, lr, scheme)
	})
}

func nonEmpty(rl corev1.ResourceList) corev1.ResourceList {
	if len(rl) == 0 {
		return nil
	}
	return rl
}

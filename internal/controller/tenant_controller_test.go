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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	platformv1alpha1 "github.com/halildogan/upo/api/v1alpha1"
	"github.com/halildogan/upo/internal/platform"
	"github.com/halildogan/upo/pkg/conditions"
)

var _ = Describe("Tenant controller", func() {
	const (
		timeout  = 20 * time.Second
		interval = 250 * time.Millisecond
	)

	It("provisions an isolated namespace with a quota and reports Ready", func() {
		tenant := &platformv1alpha1.Tenant{
			ObjectMeta: metav1.ObjectMeta{Name: "acme"},
			Spec: platformv1alpha1.TenantSpec{
				DisplayName:     "Acme Corp",
				Tier:            platformv1alpha1.TenantTierStandard,
				NamespacePrefix: "tenant",
				Quota: &platformv1alpha1.TenantQuota{
					Hard: corev1.ResourceList{
						corev1.ResourcePods: resource.MustParse("50"),
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, tenant)).To(Succeed())

		By("creating the tenant namespace")
		Eventually(func(g Gomega) {
			ns := &corev1.Namespace{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "tenant-acme"}, ns)).To(Succeed())
			g.Expect(ns.Labels).To(HaveKeyWithValue(platform.ManagedByLabel, platform.ManagedByValue))
		}, timeout, interval).Should(Succeed())

		By("creating the managed ResourceQuota")
		Eventually(func(g Gomega) {
			rq := &corev1.ResourceQuota{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "tenant-acme", Name: "upo-tenant-quota"}, rq)).To(Succeed())
			g.Expect(rq.Spec.Hard).To(HaveKey(corev1.ResourcePods))
		}, timeout, interval).Should(Succeed())

		By("reporting Active phase and Ready=True")
		Eventually(func(g Gomega) {
			got := &platformv1alpha1.Tenant{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "acme"}, got)).To(Succeed())
			g.Expect(got.Status.Namespace).To(Equal("tenant-acme"))
			g.Expect(got.Status.Phase).To(Equal(platformv1alpha1.TenantPhaseActive))
			g.Expect(conditions.IsTrue(got.Status.Conditions, platformv1alpha1.ConditionReady)).To(BeTrue())
		}, timeout, interval).Should(Succeed())
	})

	It("suspends a tenant by pinning pods to zero", func() {
		tenant := &platformv1alpha1.Tenant{
			ObjectMeta: metav1.ObjectMeta{Name: "beta"},
			Spec: platformv1alpha1.TenantSpec{
				Lifecycle: platformv1alpha1.LifecyclePolicy{Suspended: true},
			},
		}
		Expect(k8sClient.Create(ctx, tenant)).To(Succeed())

		Eventually(func(g Gomega) {
			rq := &corev1.ResourceQuota{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "tenant-beta", Name: "upo-tenant-quota"}, rq)).To(Succeed())
			pods := rq.Spec.Hard[corev1.ResourcePods]
			g.Expect(pods.String()).To(Equal("0"))

			got := &platformv1alpha1.Tenant{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "beta"}, got)).To(Succeed())
			g.Expect(got.Status.Phase).To(Equal(platformv1alpha1.TenantPhaseSuspended))
		}, timeout, interval).Should(Succeed())
	})

	It("garbage-collects the namespace on deletion", func() {
		tenant := &platformv1alpha1.Tenant{ObjectMeta: metav1.ObjectMeta{Name: "gamma"}}
		Expect(k8sClient.Create(ctx, tenant)).To(Succeed())

		Eventually(func(g Gomega) {
			ns := &corev1.Namespace{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "tenant-gamma"}, ns)).To(Succeed())
		}, timeout, interval).Should(Succeed())

		Expect(k8sClient.Delete(ctx, tenant)).To(Succeed())

		By("removing the finalizer once teardown completes")
		Eventually(func(g Gomega) {
			got := &platformv1alpha1.Tenant{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: "gamma"}, got)
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
		}, timeout, interval).Should(Succeed())
	})
})

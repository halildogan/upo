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

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	platformv1alpha1 "github.com/halildogan/upo/api/v1alpha1"
)

// DefaultNetworkPolicyName is the deterministic name of the managed default
// NetworkPolicy applied to a tenant namespace.
const DefaultNetworkPolicyName = "upo-tenant-default"

// dnsPort is the well-known DNS port permitted for egress under Baseline.
var dnsPort = intstr.FromInt32(53)

// ReconcileNetworkPolicy converges the default NetworkPolicy posture for a
// tenant namespace. The empty PodSelector ({}) selects all pods in the
// namespace; PolicyTypes determine whether ingress, egress, or both are
// default-denied unless explicitly allowed by the rules below.
//
//   - None:     the managed policy is removed (cluster default applies).
//   - Baseline: allow ingress only from the same namespace; allow DNS egress
//     plus egress to the same namespace; deny everything else.
//   - Strict:   default-deny all ingress and egress; workloads must ship their
//     own NetworkPolicies to open specific flows.
func ReconcileNetworkPolicy(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	owner client.Object,
	namespace string,
	isolation platformv1alpha1.NetworkIsolation,
	labels map[string]string,
) (controllerutil.OperationResult, error) {
	np := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: DefaultNetworkPolicyName, Namespace: namespace}}

	if isolation == platformv1alpha1.NetworkIsolationNone {
		if err := c.Delete(ctx, np); err != nil && !ignoreNotFound(err) {
			return controllerutil.OperationResultNone, err
		}
		return controllerutil.OperationResultNone, nil
	}

	return controllerutil.CreateOrUpdate(ctx, c, np, func() error {
		np.Labels = MergeLabels(np.Labels, labels)
		np.Spec.PodSelector = metav1.LabelSelector{} // all pods
		np.Spec.PolicyTypes = []networkingv1.PolicyType{
			networkingv1.PolicyTypeIngress,
			networkingv1.PolicyTypeEgress,
		}

		switch isolation {
		case platformv1alpha1.NetworkIsolationStrict:
			// No ingress/egress rules => default-deny both directions.
			np.Spec.Ingress = nil
			np.Spec.Egress = nil
		case platformv1alpha1.NetworkIsolationBaseline:
			sameNamespace := []networkingv1.NetworkPolicyPeer{{
				PodSelector: &metav1.LabelSelector{},
			}}
			np.Spec.Ingress = []networkingv1.NetworkPolicyIngressRule{{
				From: sameNamespace,
			}}
			udp := networkingv1.ProtocolUDP
			tcp := networkingv1.ProtocolTCP
			np.Spec.Egress = []networkingv1.NetworkPolicyEgressRule{
				{To: sameNamespace},
				{ // permit cluster DNS resolution
					Ports: []networkingv1.NetworkPolicyPort{
						{Protocol: &udp, Port: &dnsPort},
						{Protocol: &tcp, Port: &dnsPort},
					},
				},
			}
		}
		return controllerutil.SetControllerReference(owner, np, scheme)
	})
}

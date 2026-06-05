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
	corev1 "k8s.io/api/core/v1"

	platformv1alpha1 "github.com/halildogan/upo/api/v1alpha1"
)

// ProfileQuota translates a coarse ResourceProfile into a concrete set of
// ResourceQuota hard limits for an environment's workload namespace. Keeping the
// mapping in one place lets cluster operators retune capacity centrally without
// editing every Environment manifest.
func ProfileQuota(p platformv1alpha1.ResourceProfile) corev1.ResourceList {
	switch p {
	case platformv1alpha1.ResourceProfileSmall:
		return quota("2", "4Gi", "20")
	case platformv1alpha1.ResourceProfileLarge:
		return quota("8", "16Gi", "80")
	case platformv1alpha1.ResourceProfileXLarge:
		return quota("16", "32Gi", "160")
	case platformv1alpha1.ResourceProfileMedium:
		fallthrough
	default:
		return quota("4", "8Gi", "40")
	}
}

// quota builds a ResourceList that bounds both requests and limits for CPU and
// memory and caps the pod count, the four dimensions that matter most for
// noisy-neighbor isolation between environments.
func quota(cpu, mem, pods string) corev1.ResourceList {
	return corev1.ResourceList{
		corev1.ResourceRequestsCPU:    mustQuantity(cpu),
		corev1.ResourceLimitsCPU:      mustQuantity(cpu),
		corev1.ResourceRequestsMemory: mustQuantity(mem),
		corev1.ResourceLimitsMemory:   mustQuantity(mem),
		corev1.ResourcePods:           mustQuantity(pods),
	}
}

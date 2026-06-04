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

// Package conditions provides thin, allocation-free helpers around the
// upstream meta.SetStatusCondition machinery. Centralizing condition mutation
// guarantees every controller writes conditions that satisfy the Kubernetes API
// validation rules (non-empty Reason, observedGeneration set) and that
// LastTransitionTime only changes when Status actually flips.
package conditions

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// reasonFallback satisfies the API requirement that Condition.Reason be a
// non-empty CamelCase token even if a caller forgets to provide one.
const reasonFallback = "Unspecified"

// set is the single mutation point for all condition writes.
func set(conditions *[]metav1.Condition, condType string, status metav1.ConditionStatus, reason, message string, observedGeneration int64) {
	if reason == "" {
		reason = reasonFallback
	}
	meta.SetStatusCondition(conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: observedGeneration,
	})
}

// MarkTrue sets condType to True. Used to record that a sub-goal is satisfied.
func MarkTrue(conditions *[]metav1.Condition, condType, reason, message string, observedGeneration int64) {
	set(conditions, condType, metav1.ConditionTrue, reason, message, observedGeneration)
}

// MarkFalse sets condType to False. Used to record a blocked or failed sub-goal.
func MarkFalse(conditions *[]metav1.Condition, condType, reason, message string, observedGeneration int64) {
	set(conditions, condType, metav1.ConditionFalse, reason, message, observedGeneration)
}

// MarkUnknown sets condType to Unknown. Used while a result is still pending.
func MarkUnknown(conditions *[]metav1.Condition, condType, reason, message string, observedGeneration int64) {
	set(conditions, condType, metav1.ConditionUnknown, reason, message, observedGeneration)
}

// IsTrue reports whether the named condition is present and True.
func IsTrue(conditions []metav1.Condition, condType string) bool {
	return meta.IsStatusConditionTrue(conditions, condType)
}

// IsFalse reports whether the named condition is present and False.
func IsFalse(conditions []metav1.Condition, condType string) bool {
	return meta.IsStatusConditionFalse(conditions, condType)
}

// Get returns the named condition, or nil if it is not present.
func Get(conditions []metav1.Condition, condType string) *metav1.Condition {
	return meta.FindStatusCondition(conditions, condType)
}

// Remove deletes the named condition if present (e.g. clearing Degraded once healthy).
func Remove(conditions *[]metav1.Condition, condType string) {
	meta.RemoveStatusCondition(conditions, condType)
}

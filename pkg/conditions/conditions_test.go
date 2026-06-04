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

package conditions

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMarkAndQuery(t *testing.T) {
	var conds []metav1.Condition

	MarkTrue(&conds, "Ready", "Provisioned", "all good", 7)
	if !IsTrue(conds, "Ready") {
		t.Fatalf("expected Ready to be True")
	}
	c := Get(conds, "Ready")
	if c == nil || c.ObservedGeneration != 7 || c.Reason != "Provisioned" {
		t.Fatalf("unexpected condition: %+v", c)
	}

	// Flipping status should update it in place (single entry per type).
	MarkFalse(&conds, "Ready", "Degraded", "broke", 8)
	if IsTrue(conds, "Ready") || !IsFalse(conds, "Ready") {
		t.Fatalf("expected Ready to be False after MarkFalse")
	}
	if len(conds) != 1 {
		t.Fatalf("expected exactly one condition entry, got %d", len(conds))
	}
}

func TestReasonFallback(t *testing.T) {
	var conds []metav1.Condition
	// Empty reason would violate API validation; the helper must substitute one.
	MarkUnknown(&conds, "Progressing", "", "pending", 1)
	c := Get(conds, "Progressing")
	if c == nil || c.Reason == "" {
		t.Fatalf("expected a non-empty fallback reason, got %+v", c)
	}
}

func TestRemove(t *testing.T) {
	var conds []metav1.Condition
	MarkTrue(&conds, "Degraded", "Err", "x", 1)
	Remove(&conds, "Degraded")
	if Get(conds, "Degraded") != nil {
		t.Fatalf("expected Degraded to be removed")
	}
}

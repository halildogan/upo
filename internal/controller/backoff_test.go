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
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	platformv1alpha1 "github.com/halildogan/upo/api/v1alpha1"
)

// TestComputeBackoff is a pure unit test (no cluster required) verifying the
// retry/backoff math used by the Integration controller.
func TestComputeBackoff(t *testing.T) {
	dur := func(d time.Duration) *metav1.Duration { return &metav1.Duration{Duration: d} }

	tests := []struct {
		name    string
		policy  *platformv1alpha1.RetryPolicy
		attempt int32
		want    time.Duration
	}{
		{
			name:    "nil policy uses exponential defaults (attempt 1)",
			policy:  nil,
			attempt: 1,
			want:    defaultInitialBackoff, // 5s * 2^0
		},
		{
			name:    "nil policy doubles on attempt 3",
			policy:  nil,
			attempt: 3,
			want:    4 * defaultInitialBackoff, // 5s * 2^2
		},
		{
			name:    "fixed strategy ignores attempt",
			policy:  &platformv1alpha1.RetryPolicy{Backoff: platformv1alpha1.BackoffStrategyFixed, InitialDelay: dur(2 * time.Second)},
			attempt: 7,
			want:    2 * time.Second,
		},
		{
			name:    "exponential is capped at maxDelay",
			policy:  &platformv1alpha1.RetryPolicy{Backoff: platformv1alpha1.BackoffStrategyExponential, InitialDelay: dur(time.Second), MaxDelay: dur(10 * time.Second)},
			attempt: 20,
			want:    10 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeBackoff(tt.policy, tt.attempt)
			if got != tt.want {
				t.Fatalf("computeBackoff() = %s, want %s", got, tt.want)
			}
		})
	}
}

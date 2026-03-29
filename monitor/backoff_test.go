// Package monitor — unit tests for computeBackoffDelay.
// Covers all three strategies across multiple retry counts.
// Red criteria defined before implementation was written (TDD, TASK-010).
// See: REQ-011, TASK-010
package monitor

import (
	"testing"
	"time"

	"github.com/nxlabs/nexusflow/internal/models"
)

// TestComputeBackoffDelay_Exponential verifies that exponential backoff produces
// 1s, 2s, 4s delays for retry counts 0, 1, 2 respectively.
// Acceptance Criterion 2: "Backoff delay applied between retries (exponential: 1s, 2s, 4s)".
func TestComputeBackoffDelay_Exponential(t *testing.T) {
	cases := []struct {
		retryCount int
		want       time.Duration
	}{
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
	}

	for _, tc := range cases {
		got := computeBackoffDelay(models.BackoffExponential, tc.retryCount)
		if got != tc.want {
			t.Errorf("computeBackoffDelay(exponential, retryCount=%d) = %v, want %v",
				tc.retryCount, got, tc.want)
		}
	}
}

// TestComputeBackoffDelay_Linear verifies that linear backoff produces
// 1s, 2s, 3s delays for retry counts 0, 1, 2 respectively.
func TestComputeBackoffDelay_Linear(t *testing.T) {
	cases := []struct {
		retryCount int
		want       time.Duration
	}{
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 3 * time.Second},
	}

	for _, tc := range cases {
		got := computeBackoffDelay(models.BackoffLinear, tc.retryCount)
		if got != tc.want {
			t.Errorf("computeBackoffDelay(linear, retryCount=%d) = %v, want %v",
				tc.retryCount, got, tc.want)
		}
	}
}

// TestComputeBackoffDelay_Fixed verifies that fixed backoff always returns 1s
// regardless of the retry count.
func TestComputeBackoffDelay_Fixed(t *testing.T) {
	for _, retryCount := range []int{0, 1, 5, 10} {
		got := computeBackoffDelay(models.BackoffFixed, retryCount)
		if got != time.Second {
			t.Errorf("computeBackoffDelay(fixed, retryCount=%d) = %v, want 1s",
				retryCount, got)
		}
	}
}

// TestComputeBackoffDelay_UnknownStrategyFallsBackToFixed verifies that an
// unrecognised strategy defaults to a safe 1s delay rather than zero (fail-safe).
func TestComputeBackoffDelay_UnknownStrategyFallsBackToFixed(t *testing.T) {
	got := computeBackoffDelay("unknown-strategy", 2)
	if got != time.Second {
		t.Errorf("computeBackoffDelay(unknown, 2) = %v, want 1s (fail-safe fallback)", got)
	}
}

// TestComputeBackoffDelay_NeverReturnsZero verifies the postcondition that the
// returned delay is always positive for all supported strategies.
func TestComputeBackoffDelay_NeverReturnsZero(t *testing.T) {
	strategies := []models.BackoffStrategy{
		models.BackoffExponential,
		models.BackoffLinear,
		models.BackoffFixed,
	}
	for _, strategy := range strategies {
		for _, retryCount := range []int{0, 1, 2, 3} {
			got := computeBackoffDelay(strategy, retryCount)
			if got <= 0 {
				t.Errorf("computeBackoffDelay(%s, %d) = %v, want positive duration",
					strategy, retryCount, got)
			}
		}
	}
}

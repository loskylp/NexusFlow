// Package monitor — backoff delay computation for task retry.
// Implements exponential, linear, and fixed backoff strategies as defined by
// the RetryConfig on each Task. Used by the Monitor to gate re-enqueue timing
// after XCLAIM reclamation of tasks from downed workers.
// See: REQ-011, TASK-010, ADR-002
package monitor

import (
	"time"

	"github.com/nxlabs/nexusflow/internal/models"
)

// baseRetryDelay is the unit of time for all backoff strategies.
// Exponential: baseRetryDelay * 2^retryCount  (1s, 2s, 4s, 8s, …)
// Linear:      baseRetryDelay * (retryCount+1) (1s, 2s, 3s, 4s, …)
// Fixed:       baseRetryDelay                  (always 1s)
const baseRetryDelay = time.Second

// computeBackoffDelay returns the duration to wait before a task may be
// re-enqueued after its retryCount-th failure.
//
// retryCount is the zero-based attempt index: 0 for the first retry, 1 for
// the second, etc. The delay is computed before IncrementRetryCount is
// applied, so retryCount reflects the number of attempts already made.
//
// Exponential: 1s * 2^retryCount  → 1s, 2s, 4s, 8s, …
// Linear:      1s * (retryCount+1) → 1s, 2s, 3s, 4s, …
// Fixed:       1s regardless of retryCount
//
// An unrecognised strategy falls back to fixed (fail-safe: 1s delay is always
// safe; zero delay would violate the backoff contract).
//
// Args:
//
//	strategy:   The BackoffStrategy declared in the Task's RetryConfig.
//	retryCount: The number of retries already performed (pre-increment value).
//
// Returns:
//
//	A positive duration. Never zero.
func computeBackoffDelay(strategy models.BackoffStrategy, retryCount int) time.Duration {
	switch strategy {
	case models.BackoffExponential:
		// 1s * 2^retryCount. Cap the shift at 30 to prevent overflow on
		// pathological retry counts; 2^30 seconds ≈ 34 years.
		shift := retryCount
		if shift > 30 {
			shift = 30
		}
		return baseRetryDelay * time.Duration(1<<uint(shift))

	case models.BackoffLinear:
		return baseRetryDelay * time.Duration(retryCount+1)

	case models.BackoffFixed:
		return baseRetryDelay

	default:
		// Unknown strategy: fall back to fixed so the task is never blocked
		// indefinitely by a misconfigured strategy.
		return baseRetryDelay
	}
}

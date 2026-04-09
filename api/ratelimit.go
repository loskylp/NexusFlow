// Package api — login rate limiter (SEC-003).
// Tracks failed login attempts per IP address and enforces a lockout after a
// configurable number of consecutive failures within a rolling window.
//
// Design constraints (SEC-003):
//   - Only FAILED attempts (401 responses) count toward the limit.
//   - A successful login (200) resets the counter for that IP.
//   - After maxFailures consecutive failures the IP is locked out for lockoutDuration.
//   - Locked-out IPs receive HTTP 429 with a Retry-After: 60 header.
//   - State is in-memory only; no external store is needed.
//   - Expired entries are evicted lazily on the next access from the same IP.
package api

import (
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// ipRecord holds the failure state for a single IP address.
type ipRecord struct {
	failures  int
	lockedAt  time.Time // zero value means no active lockout
}

// LoginRateLimiter tracks failed login attempts per IP and enforces lockout.
// All exported methods are safe for concurrent use.
type LoginRateLimiter struct {
	mu             sync.Mutex
	records        map[string]*ipRecord
	maxFailures    int
	lockoutDuration time.Duration
}

// NewLoginRateLimiter constructs a LoginRateLimiter.
//
// Parameters:
//   - maxFailures:      number of consecutive failures before lockout is applied.
//   - lockoutDuration:  how long the IP remains locked after reaching maxFailures.
//
// Preconditions:
//   - maxFailures >= 1
//   - lockoutDuration > 0
func NewLoginRateLimiter(maxFailures int, lockoutDuration time.Duration) *LoginRateLimiter {
	return &LoginRateLimiter{
		records:         make(map[string]*ipRecord),
		maxFailures:     maxFailures,
		lockoutDuration: lockoutDuration,
	}
}

// isLocked reports whether the IP is currently locked out.
// Expired lockouts are cleared lazily; the record is mutated in place.
//
// Preconditions:
//   - rl.mu must be held by the caller.
func (rl *LoginRateLimiter) isLocked(rec *ipRecord) bool {
	if rec.lockedAt.IsZero() {
		return false
	}
	if time.Since(rec.lockedAt) >= rl.lockoutDuration {
		// Lockout has expired — reset the record entirely.
		rec.failures = 0
		rec.lockedAt = time.Time{}
		return false
	}
	return true
}

// recordFailure increments the failure counter for ip and applies a lockout
// when maxFailures is reached.
//
// Postconditions:
//   - The in-memory record for ip reflects the incremented failure count.
//   - If failures reaches maxFailures, lockedAt is set to the current time.
func (rl *LoginRateLimiter) recordFailure(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rec := rl.getOrCreate(ip)
	if rl.isLocked(rec) {
		// Already locked; no need to increment.
		return
	}
	rec.failures++
	if rec.failures >= rl.maxFailures {
		rec.lockedAt = time.Now()
	}
}

// resetCounter clears all failure state for ip.
// Called after a successful login so the IP can attempt further logins.
func (rl *LoginRateLimiter) resetCounter(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	delete(rl.records, ip)
}

// checkLocked reports whether ip is currently locked out without modifying state.
// Returns true if the IP must be refused; false if the request can proceed.
func (rl *LoginRateLimiter) checkLocked(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rec, ok := rl.records[ip]
	if !ok {
		return false
	}
	return rl.isLocked(rec)
}

// getOrCreate returns the existing record for ip or inserts and returns a new one.
//
// Preconditions:
//   - rl.mu must be held by the caller.
func (rl *LoginRateLimiter) getOrCreate(ip string) *ipRecord {
	rec, ok := rl.records[ip]
	if !ok {
		rec = &ipRecord{}
		rl.records[ip] = rec
	}
	return rec
}

// Middleware wraps an http.HandlerFunc with login rate-limit enforcement.
// The wrapped handler is called only when the IP is not locked out.
// After the inner handler returns, the response status determines whether to
// record a failure (401) or reset the counter (200).
//
// Responses added by the middleware:
//   - HTTP 429 with Retry-After header when the IP is locked out.
//
// The inner handler's response codes drive counter updates:
//   - 200: counter reset for the IP.
//   - 401: failure recorded; lockout applied if threshold is reached.
//   - Other codes: counter is not modified.
func (rl *LoginRateLimiter) Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := extractIP(r)

		if rl.checkLocked(ip) {
			retryAfterSeconds := int(rl.lockoutDuration.Seconds())
			w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds))
			http.Error(w, "too many failed login attempts", http.StatusTooManyRequests)
			return
		}

		// Capture the response status written by the inner handler.
		rw := &statusRecorder{ResponseWriter: w}
		next(rw, r)

		switch rw.status {
		case http.StatusOK:
			rl.resetCounter(ip)
		case http.StatusUnauthorized:
			rl.recordFailure(ip)
		}
	}
}

// extractIP returns the client IP address from r.RemoteAddr, stripping the port.
// Falls back to the raw RemoteAddr string if parsing fails.
func extractIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// RemoteAddr without a port (unlikely in practice) — use it as-is.
		return r.RemoteAddr
	}
	return host
}

// statusRecorder is a minimal http.ResponseWriter wrapper that captures the
// HTTP status code written by the inner handler so the rate limiter can
// inspect it after the handler returns.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

// WriteHeader captures the status code and forwards the call to the underlying writer.
func (sr *statusRecorder) WriteHeader(code int) {
	sr.status = code
	sr.ResponseWriter.WriteHeader(code)
}

// Write forwards to the underlying writer. If WriteHeader has not been called
// yet, it records StatusOK as the implicit status before writing.
func (sr *statusRecorder) Write(b []byte) (int, error) {
	if sr.status == 0 {
		sr.status = http.StatusOK
	}
	return sr.ResponseWriter.Write(b)
}

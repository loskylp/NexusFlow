// Package api — unit tests for login rate limiter (SEC-003).
// Verifies that failed login attempts from the same IP are tracked and that
// the IP is locked out after 3 failures for 1 minute.
package api

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/nxlabs/nexusflow/internal/models"
)

// --- helpers ---

// rateLimitedLoginHandler wires the rate limiter around the auth Login handler
// the same way server.go does, so tests exercise the full stack.
func rateLimitedLoginHandler(srv *Server, rl *LoginRateLimiter) http.HandlerFunc {
	authH := &AuthHandler{server: srv}
	return rl.Middleware(authH.Login)
}

// loginRequestWithIP returns a POST /api/auth/login request with RemoteAddr set.
func loginRequestWithIP(username, password, remoteAddr string) *http.Request {
	req := postLoginRequest(username, password)
	req.RemoteAddr = remoteAddr
	return req
}

// --- tests ---

// TestRateLimit_ThreeFailedAttemptsTriggersLockout verifies that after 3 failed
// attempts from the same IP the next (4th) attempt is refused with HTTP 429.
// The 3 accumulating failures themselves return 401 — the lockout activates
// for all subsequent requests once the threshold is crossed.
func TestRateLimit_ThreeFailedAttemptsTriggersLockout(t *testing.T) {
	users := newStubUserRepo()
	srv := testServer(users, newStubSessionStore())
	rl := NewLoginRateLimiter(3, time.Minute)
	h := rateLimitedLoginHandler(srv, rl)

	// Three failures accumulate the lockout.
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, loginRequestWithIP("nobody", "bad", "1.2.3.4:5000"))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i+1, rec.Code)
		}
	}

	// Fourth attempt must be refused with 429.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, loginRequestWithIP("nobody", "bad", "1.2.3.4:5000"))
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("4th attempt after 3 failures: expected 429, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestRateLimit_FourthAttemptDuringLockoutReturns429 verifies that once locked
// out, subsequent attempts are blocked even before another credential check.
func TestRateLimit_FourthAttemptDuringLockoutReturns429(t *testing.T) {
	users := newStubUserRepo()
	srv := testServer(users, newStubSessionStore())
	rl := NewLoginRateLimiter(3, time.Minute)
	h := rateLimitedLoginHandler(srv, rl)

	// Exhaust attempts to trigger lockout.
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, loginRequestWithIP("nobody", "bad", "10.0.0.1:9000"))
	}

	// Fourth attempt must also return 429.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, loginRequestWithIP("nobody", "bad", "10.0.0.1:9000"))
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("4th attempt: expected 429, got %d", rec.Code)
	}
}

// TestRateLimit_RetryAfterHeaderPresent verifies that 429 responses include the
// Retry-After header set to 60 seconds.
func TestRateLimit_RetryAfterHeaderPresent(t *testing.T) {
	users := newStubUserRepo()
	srv := testServer(users, newStubSessionStore())
	rl := NewLoginRateLimiter(3, time.Minute)
	h := rateLimitedLoginHandler(srv, rl)

	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, loginRequestWithIP("nobody", "bad", "5.5.5.5:1234"))
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, loginRequestWithIP("nobody", "bad", "5.5.5.5:1234"))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "60" {
		t.Errorf("expected Retry-After: 60, got %q", got)
	}
}

// TestRateLimit_SuccessfulLoginResetsCounter verifies that a successful login
// clears the failure counter, allowing the IP to attempt login again later.
func TestRateLimit_SuccessfulLoginResetsCounter(t *testing.T) {
	users := newStubUserRepo()
	sessions := newStubSessionStore()
	u := activeUserWithPassword(t, "dave", "correct", models.RoleUser)
	users.addUser(u)

	srv := testServer(users, sessions)
	rl := NewLoginRateLimiter(3, time.Minute)
	h := rateLimitedLoginHandler(srv, rl)

	ip := "7.7.7.7:8888"

	// Two failures.
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, loginRequestWithIP("dave", "wrong", ip))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("failure %d: expected 401, got %d", i+1, rec.Code)
		}
	}

	// Successful login must reset the counter.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, loginRequestWithIP("dave", "correct", ip))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on success, got %d: %s", rec.Code, rec.Body.String())
	}

	// Two more failures after the reset must NOT trigger lockout (counter reset to 0).
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, loginRequestWithIP("dave", "wrong", ip))
		if rec.Code == http.StatusTooManyRequests {
			t.Errorf("unexpected 429 after counter reset (attempt %d after reset)", i+1)
		}
	}
}

// TestRateLimit_LockoutExpiresAfterWindow verifies that after the lockout
// duration has elapsed, the IP can attempt login again.
func TestRateLimit_LockoutExpiresAfterWindow(t *testing.T) {
	users := newStubUserRepo()
	srv := testServer(users, newStubSessionStore())

	// Use a very short lockout for test speed.
	shortWindow := 50 * time.Millisecond
	rl := NewLoginRateLimiter(3, shortWindow)
	h := rateLimitedLoginHandler(srv, rl)

	ip := "9.9.9.9:4444"

	// Trigger lockout.
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, loginRequestWithIP("nobody", "bad", ip))
	}

	// Confirm lockout is active.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, loginRequestWithIP("nobody", "bad", ip))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 during lockout, got %d", rec.Code)
	}

	// Wait for lockout to expire.
	time.Sleep(shortWindow + 10*time.Millisecond)

	// After expiry the IP should be allowed to try again (returns 401, not 429).
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, loginRequestWithIP("nobody", "bad", ip))
	if rec.Code == http.StatusTooManyRequests {
		t.Errorf("expected non-429 after lockout expiry, got 429")
	}
}

// TestRateLimit_DifferentIPsAreTrackedSeparately verifies that the rate limiter
// does not share failure counts across distinct IPs.
func TestRateLimit_DifferentIPsAreTrackedSeparately(t *testing.T) {
	users := newStubUserRepo()
	srv := testServer(users, newStubSessionStore())
	rl := NewLoginRateLimiter(3, time.Minute)
	h := rateLimitedLoginHandler(srv, rl)

	// Exhaust attempts for IP A.
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, loginRequestWithIP("nobody", "bad", "192.168.1.1:1111"))
	}

	// IP B must not be locked out.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, loginRequestWithIP("nobody", "bad", "192.168.1.2:1111"))
	if rec.Code == http.StatusTooManyRequests {
		t.Errorf("IP B should not be locked out, but got 429")
	}
}

// TestRateLimit_ConcurrentRequestsDoNotRaceOnCounter verifies that concurrent
// failed login attempts from the same IP do not corrupt the internal counter.
// This is a basic race-safety check; run with -race to detect data races.
func TestRateLimit_ConcurrentRequestsDoNotRaceOnCounter(t *testing.T) {
	users := newStubUserRepo()
	srv := testServer(users, newStubSessionStore())
	rl := NewLoginRateLimiter(3, time.Minute)
	h := rateLimitedLoginHandler(srv, rl)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, loginRequestWithIP("nobody", "bad", "3.3.3.3:9999"))
		}()
	}
	wg.Wait()
	// No assertion on exact counts — the goal is no panic and no data race.
}

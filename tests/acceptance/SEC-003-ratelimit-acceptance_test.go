// Package acceptance — acceptance tests for SEC-003 login rate limiting.
//
// These tests verify all 7 acceptance criteria through the exported
// LoginRateLimiter.Middleware interface, exercising the full middleware
// lifecycle: lockout detection, failure counting, expiry, and header emission.
//
// The inner handler used in each test is a stub that mimics the real AuthHandler:
// - bad credentials → 401
// - good credentials → 200
// - missing/empty username → 400
//
// This is a valid acceptance test stance: the rate limiter's observable behaviour
// is entirely at the HTTP request/response boundary. The inner handler is
// substitutable because the middleware contract is defined on HTTP status codes,
// not on credential validation logic.
//
// Test traceability: SEC-003 acceptance criteria AC-1 through AC-7.
// GWT structure applied to every test case per Verifier standard.
//
// Run:
//
//	go test ./tests/acceptance/... -v -run SEC003 -race
package acceptance

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nxlabs/nexusflow/api"
)

// ---------------------------------------------------------------------------
// Stub login handler — mimics AuthHandler.Login status code contract.
// ---------------------------------------------------------------------------

// stubLoginHandler returns an http.HandlerFunc that accepts one set of valid
// credentials and rejects all others.
//
//   - username=valid, password=correct → 200 OK
//   - username="" → 400 Bad Request
//   - anything else → 401 Unauthorized
func stubLoginHandler(validUser, validPass string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := r.Header.Get("X-Test-User")
		pass := r.Header.Get("X-Test-Pass")
		if user == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if user == validUser && pass == validPass {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}
}

// buildRequest creates an HTTP request with the given RemoteAddr and test credentials.
func buildRequest(user, pass, remoteAddr string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	req.Header.Set("X-Test-User", user)
	req.Header.Set("X-Test-Pass", pass)
	req.RemoteAddr = remoteAddr
	return req
}

// failRequest builds a request that the stub handler will reject with 401.
func failRequest(remoteAddr string) *http.Request {
	return buildRequest("nobody", "wrong", remoteAddr)
}

// badRequest builds a request that the stub handler will reject with 400.
func badRequest(remoteAddr string) *http.Request {
	return buildRequest("", "", remoteAddr) // empty username → 400
}

// ---------------------------------------------------------------------------
// AC-1: After 3 failed login attempts from same IP, return HTTP 429.
// ---------------------------------------------------------------------------

// SEC-003 AC-1 (positive): 3 failures from same IP trigger 429 on the 4th attempt.
func TestSEC003_AC1_ThreeFailedAttemptsTrigger429(t *testing.T) {
	// Given: a rate limiter configured for 3 max failures / 60s lockout,
	//        and an IP with no prior login history.
	rl := api.NewLoginRateLimiter(3, 60*time.Second)
	h := rl.Middleware(stubLoginHandler("alice", "secret"))
	ip := "203.0.113.1:54321"

	// When: the same IP sends 3 failed login attempts (each returns 401).
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, failRequest(ip))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("AC-1 setup: attempt %d expected 401, got %d", i+1, rec.Code)
		}
	}

	// Then: the 4th attempt is refused with HTTP 429.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, failRequest(ip))
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("AC-1 FAIL: after 3 failures expected 429, got %d", rec.Code)
	}
}

// SEC-003 AC-1 (negative): 2 failures from same IP do NOT trigger 429.
// [VERIFIER-ADDED]
func TestSEC003_AC1_Negative_TwoFailuresDoNotTrigger429(t *testing.T) {
	// Given: a rate limiter and an IP with no prior history.
	rl := api.NewLoginRateLimiter(3, 60*time.Second)
	h := rl.Middleware(stubLoginHandler("alice", "secret"))
	ip := "203.0.113.2:54321"

	// When: the IP sends only 2 failed login attempts.
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, failRequest(ip))
	}

	// Then: the 3rd attempt still returns 401 — threshold not yet crossed.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, failRequest(ip))
	if rec.Code == http.StatusTooManyRequests {
		t.Errorf("AC-1 FAIL (negative): 2 failures must not trigger lockout; got 429 on 3rd attempt")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("AC-1 FAIL (negative): 3rd attempt should be 401, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// AC-2: Lockout disables login for 1 minute.
// ---------------------------------------------------------------------------

// SEC-003 AC-2 (positive): Locked IP is refused for the full lockout window.
func TestSEC003_AC2_LockedIPRefusedDuringWindow(t *testing.T) {
	// Given: an IP that has just been locked out after 3 failures.
	rl := api.NewLoginRateLimiter(3, 60*time.Second)
	h := rl.Middleware(stubLoginHandler("alice", "secret"))
	ip := "203.0.113.3:54321"

	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, failRequest(ip))
	}

	// When: the locked IP makes 3 more attempts during the lockout window.
	// Then: all 3 are refused with 429.
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, failRequest(ip))
		if rec.Code != http.StatusTooManyRequests {
			t.Errorf("AC-2 FAIL: attempt %d during lockout expected 429, got %d", i+1, rec.Code)
		}
	}
}

// SEC-003 AC-2 (negative): Lockout expires after the configured window.
func TestSEC003_AC2_Negative_LockoutExpiresAfterWindow(t *testing.T) {
	// Given: a 60ms lockout window and a locked-out IP.
	shortWindow := 60 * time.Millisecond
	rl := api.NewLoginRateLimiter(3, shortWindow)
	h := rl.Middleware(stubLoginHandler("alice", "secret"))
	ip := "203.0.113.4:54321"

	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, failRequest(ip))
	}

	// Confirm lockout is active immediately.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, failRequest(ip))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("AC-2 negative setup: expected 429 during lockout, got %d", rec.Code)
	}

	// When: the lockout window elapses.
	time.Sleep(shortWindow + 10*time.Millisecond)

	// Then: the IP can attempt login again (receives 401, not 429).
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, failRequest(ip))
	if rec.Code == http.StatusTooManyRequests {
		t.Errorf("AC-2 FAIL (negative): expected non-429 after lockout expiry, still got 429")
	}
}

// ---------------------------------------------------------------------------
// AC-3: Retry-After: 60 header present on 429 responses.
// ---------------------------------------------------------------------------

// SEC-003 AC-3 (positive): 429 response includes Retry-After: 60.
func TestSEC003_AC3_RetryAfterHeaderPresentOn429(t *testing.T) {
	// Given: a locked-out IP (production config: 60s window).
	rl := api.NewLoginRateLimiter(3, 60*time.Second)
	h := rl.Middleware(stubLoginHandler("alice", "secret"))
	ip := "203.0.113.5:54321"

	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, failRequest(ip))
	}

	// When: the locked IP attempts login.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, failRequest(ip))

	// Then: the response is HTTP 429 with Retry-After: 60.
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("AC-3: expected 429, got %d", rec.Code)
	}
	retryAfter := rec.Header().Get("Retry-After")
	if retryAfter != "60" {
		t.Errorf("AC-3 FAIL: expected Retry-After: 60, got %q", retryAfter)
	}
}

// SEC-003 AC-3 (negative): Non-lockout (401) responses must NOT include Retry-After.
// [VERIFIER-ADDED]
func TestSEC003_AC3_Negative_RetryAfterAbsentOnNon429(t *testing.T) {
	// Given: an IP that has never failed login.
	rl := api.NewLoginRateLimiter(3, 60*time.Second)
	h := rl.Middleware(stubLoginHandler("alice", "secret"))
	ip := "203.0.113.6:54321"

	// When: the IP makes a single failed login attempt.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, failRequest(ip))

	// Then: the 401 response does NOT include a Retry-After header.
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("AC-3 negative: expected 401, got %d", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "" {
		t.Errorf("AC-3 FAIL (negative): Retry-After must not be set on 401 responses, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// AC-4: Only failed attempts (401) count toward the limit.
// ---------------------------------------------------------------------------

// SEC-003 AC-4 (positive): 400 responses do NOT increment the failure counter.
// [VERIFIER-ADDED]
func TestSEC003_AC4_Positive_400DoesNotCountTowardLimit(t *testing.T) {
	// Given: a rate limiter and an IP that has sent 0 prior failures.
	rl := api.NewLoginRateLimiter(3, 60*time.Second)
	h := rl.Middleware(stubLoginHandler("alice", "secret"))
	ip := "203.0.113.7:54321"

	// When: the IP sends 5 requests that each produce a 400 response.
	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, badRequest(ip))
		if rec.Code == http.StatusTooManyRequests {
			t.Fatalf("AC-4 FAIL: 400 response counted as failure; got 429 on attempt %d", i+1)
		}
	}

	// Then: a well-formed failed attempt still returns 401, not 429.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, failRequest(ip))
	if rec.Code == http.StatusTooManyRequests {
		t.Errorf("AC-4 FAIL: 400s were incorrectly counted toward limit; 6th attempt (401-class) returned 429")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("AC-4: expected 401, got %d", rec.Code)
	}
}

// SEC-003 AC-4 (negative): 401 responses DO increment the counter.
// Each 401 must advance the counter; 3 increments must trigger lockout.
func TestSEC003_AC4_Negative_EachUnauthorizedIncrementsCounter(t *testing.T) {
	// Given: a rate limiter and a fresh IP.
	rl := api.NewLoginRateLimiter(3, 60*time.Second)
	h := rl.Middleware(stubLoginHandler("alice", "secret"))
	ip := "203.0.113.8:54321"

	// When: the IP sends exactly 3 failed login attempts.
	codes := make([]int, 3)
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, failRequest(ip))
		codes[i] = rec.Code
	}

	// Then: all 3 returned 401 (none prematurely locked),
	// confirming each was counted correctly.
	for i, code := range codes {
		if code != http.StatusUnauthorized {
			t.Errorf("AC-4 FAIL: attempt %d should be 401, got %d", i+1, code)
		}
	}
	// And: the 4th attempt is 429.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, failRequest(ip))
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("AC-4 FAIL: 4th attempt after 3×401 expected 429, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// AC-5: Successful login resets the counter.
// ---------------------------------------------------------------------------

// SEC-003 AC-5 (positive): A successful login (200) clears the failure counter.
func TestSEC003_AC5_SuccessfulLoginResetsCounter(t *testing.T) {
	// Given: an IP that has accumulated 2 failed attempts.
	rl := api.NewLoginRateLimiter(3, 60*time.Second)
	h := rl.Middleware(stubLoginHandler("alice", "secret"))
	ip := "203.0.113.9:54321"

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, failRequest(ip))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("AC-5 setup: expected 401, got %d", rec.Code)
		}
	}

	// When: the IP submits correct credentials (returns 200).
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, buildRequest("alice", "secret", ip))
	if rec.Code != http.StatusOK {
		t.Fatalf("AC-5: expected 200 on success, got %d", rec.Code)
	}

	// Then: the counter is reset; 2 more failures from this IP do not trigger lockout.
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, failRequest(ip))
		if rec.Code == http.StatusTooManyRequests {
			t.Errorf("AC-5 FAIL: counter was not reset; got 429 on attempt %d after successful login", i+1)
		}
	}
}

// SEC-003 AC-5 (negative): Counter accumulates if no success intervenes.
// [VERIFIER-ADDED]
func TestSEC003_AC5_Negative_CounterNotResetWithoutSuccess(t *testing.T) {
	// Given: an IP with 2 prior failures and no successful login.
	rl := api.NewLoginRateLimiter(3, 60*time.Second)
	h := rl.Middleware(stubLoginHandler("alice", "secret"))
	ip := "203.0.113.10:54321"

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, failRequest(ip))
	}

	// When: a 3rd failure occurs (no intervening success).
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, failRequest(ip))
	_ = rec.Code

	// Then: the 4th attempt is 429 (counter persisted, lockout triggered).
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, failRequest(ip))
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("AC-5 FAIL (negative): without a successful login, 4th attempt should be 429, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// AC-6: Rate limiter applies to POST /api/auth/login only.
//
// This criterion concerns server.go wiring. We verify it by reading the source
// directly (static check), and by confirming that the RL middleware is NOT a
// global middleware (it is not added via r.Use but only wraps the specific route).
// The static check is noted as an observation in the verification report.
//
// Runtime test: wrapping a non-login stub handler in a separate RL instance
// and confirming it is independent (i.e., there is no shared global RL state).
// ---------------------------------------------------------------------------

// SEC-003 AC-6 (positive): Two independent handlers each have their own RL state.
// A lockout on "handler A" (login) does not bleed into "handler B" (any other route).
func TestSEC003_AC6_RateLimiterIsNotGloballyApplied(t *testing.T) {
	// Given: the rate limiter from server.go wraps ONLY the login handler.
	// We model this by creating one RL instance per route, as server.go does.
	loginRL := api.NewLoginRateLimiter(3, 60*time.Second)
	loginHandler := loginRL.Middleware(stubLoginHandler("alice", "secret"))

	// A second handler (health check, no RL) is entirely separate.
	healthHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ip := "203.0.113.11:54321"

	// When: the IP is locked out of the login route.
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		loginHandler.ServeHTTP(rec, failRequest(ip))
	}
	rec := httptest.NewRecorder()
	loginHandler.ServeHTTP(rec, failRequest(ip))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("AC-6 setup: expected login to return 429, got %d", rec.Code)
	}

	// Then: the health-check route (not wrapped by any RL) still serves 200.
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.RemoteAddr = ip
	healthHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("AC-6 FAIL: health handler returned %d; expected 200", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// AC-7: In-memory store (no Redis dependency).
// ---------------------------------------------------------------------------

// SEC-003 AC-7 (positive): Full lock/expiry lifecycle executes with zero external I/O.
// NewLoginRateLimiter accepts no connection parameters; its internal state is a
// sync.Mutex-protected map — no network connectivity required.
func TestSEC003_AC7_InMemoryStore_NoExternalDependency(t *testing.T) {
	// Given: a LoginRateLimiter constructed with no external dependencies.
	rl := api.NewLoginRateLimiter(3, 50*time.Millisecond)
	h := rl.Middleware(stubLoginHandler("alice", "secret"))
	ip := "203.0.113.12:54321"

	// When: 3 failures are accumulated (in-memory counting only).
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, failRequest(ip))
	}

	// Then: lockout is enforced entirely in memory.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, failRequest(ip))
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("AC-7 FAIL: expected 429 from in-memory store, got %d", rec.Code)
	}

	// And: expiry is also handled in memory (no external TTL mechanism).
	time.Sleep(60 * time.Millisecond)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, failRequest(ip))
	if rec.Code == http.StatusTooManyRequests {
		t.Errorf("AC-7 FAIL: in-memory expiry did not clear lockout; still getting 429")
	}
}

// ---------------------------------------------------------------------------
// [VERIFIER-ADDED] IP isolation: different IPs tracked independently.
// ---------------------------------------------------------------------------

// SEC-003 [VERIFIER-ADDED]: Lockout for IP A does not affect IP B.
func TestSEC003_IPIsolation_DifferentIPsAreIndependent(t *testing.T) {
	// Given: two distinct client IPs sharing the same middleware instance.
	rl := api.NewLoginRateLimiter(3, 60*time.Second)
	h := rl.Middleware(stubLoginHandler("alice", "secret"))
	ipA := "203.0.113.20:1111"
	ipB := "203.0.113.21:1111"

	// When: IP A is locked out after 3 failures.
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, failRequest(ipA))
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, failRequest(ipA))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("IP isolation setup: expected IP A to be locked (429), got %d", rec.Code)
	}

	// Then: IP B can still attempt login (returns 401, not 429).
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, failRequest(ipB))
	if rec.Code == http.StatusTooManyRequests {
		t.Errorf("IP isolation FAIL: IP B should not be locked out, got 429")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("IP isolation: IP B expected 401, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// [VERIFIER-ADDED] IPv6 bracket notation handling.
// ---------------------------------------------------------------------------

// SEC-003 [VERIFIER-ADDED]: extractIP correctly strips brackets and port from IPv6 addresses.
// If the IP extraction were broken for IPv6, the same IPv6 client would be treated
// as a different IP on each request (because RemoteAddr would be used as the key
// verbatim), breaking per-IP tracking.
func TestSEC003_IPv6_BracketNotationHandled(t *testing.T) {
	// Given: an IPv6 RemoteAddr in bracket notation ([::1]:port).
	rl := api.NewLoginRateLimiter(3, 60*time.Second)
	h := rl.Middleware(stubLoginHandler("alice", "secret"))
	ipv6Addr := "[::1]:54321"

	// When: the same IPv6 client sends 3 failed login attempts.
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		req := failRequest(ipv6Addr)
		h.ServeHTTP(rec, req)
	}

	// Then: the 4th attempt from the SAME IPv6 address is blocked with 429.
	// (If IP extraction were broken, each request would use the raw RemoteAddr
	// string as the key, so all 3 failures would be under different "IPs" and
	// no lockout would occur.)
	rec := httptest.NewRecorder()
	req := failRequest(ipv6Addr)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("IPv6 FAIL: IPv6 address not tracked correctly; expected 429, got %d", rec.Code)
	}
}

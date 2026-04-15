// Package auth — unit tests for auth functions and middleware.
// Tests are written first (red) against the contract defined by the scaffold stubs.
// See: ADR-006, TASK-003
package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/models"
)

// --- HashPassword and VerifyPassword ---

func TestHashPassword_ProducesVerifiableHash(t *testing.T) {
	hash, err := HashPassword("secret")
	if err != nil {
		t.Fatalf("HashPassword: unexpected error: %v", err)
	}
	if hash == "" {
		t.Fatal("HashPassword: returned empty hash")
	}
	if err := VerifyPassword("secret", hash); err != nil {
		t.Errorf("VerifyPassword: expected nil for matching password, got %v", err)
	}
}

func TestHashPassword_DifferentCallsProduceDifferentHashes(t *testing.T) {
	// bcrypt salts ensure the same password never produces the same hash.
	h1, _ := HashPassword("same")
	h2, _ := HashPassword("same")
	if h1 == h2 {
		t.Error("HashPassword: two hashes for the same password should differ (salt)")
	}
}

func TestVerifyPassword_WrongPasswordReturnsErrInvalidCredentials(t *testing.T) {
	hash, _ := HashPassword("correct")
	err := VerifyPassword("wrong", hash)
	if err == nil {
		t.Fatal("VerifyPassword: expected error for wrong password, got nil")
	}
	if err != ErrInvalidCredentials {
		t.Errorf("VerifyPassword: expected ErrInvalidCredentials, got %v", err)
	}
}

func TestVerifyPassword_EmptyPasswordReturnsError(t *testing.T) {
	hash, _ := HashPassword("correct")
	err := VerifyPassword("", hash)
	if err == nil {
		t.Fatal("VerifyPassword: expected error for empty password, got nil")
	}
}

// --- GenerateToken ---

func TestGenerateToken_Returns64CharHexString(t *testing.T) {
	token, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: unexpected error: %v", err)
	}
	if len(token) != 64 {
		t.Errorf("GenerateToken: expected 64 chars, got %d", len(token))
	}
	for _, c := range token {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("GenerateToken: non-hex character %q in token", c)
			break
		}
	}
}

func TestGenerateToken_TwoCallsProduceDifferentTokens(t *testing.T) {
	t1, _ := GenerateToken()
	t2, _ := GenerateToken()
	if t1 == t2 {
		t.Error("GenerateToken: two calls should not produce the same token")
	}
}

// --- Middleware ---

// stubSessionStore is a minimal in-memory SessionStore for middleware tests.
type stubSessionStore struct {
	sessions map[string]*models.Session
}

func newStubSessionStore() *stubSessionStore {
	return &stubSessionStore{sessions: make(map[string]*models.Session)}
}

func (s *stubSessionStore) Create(_ context.Context, token string, session *models.Session) error {
	s.sessions[token] = session
	return nil
}

func (s *stubSessionStore) Get(_ context.Context, token string) (*models.Session, error) {
	return s.sessions[token], nil
}

func (s *stubSessionStore) Delete(_ context.Context, token string) error {
	delete(s.sessions, token)
	return nil
}

func (s *stubSessionStore) DeleteAllForUser(_ context.Context, userID string) error {
	for token, sess := range s.sessions {
		if sess.UserID.String() == userID {
			delete(s.sessions, token)
		}
	}
	return nil
}

func validSession() *models.Session {
	return &models.Session{
		UserID:    uuid.New(),
		Role:      models.RoleUser,
		CreatedAt: time.Now(),
	}
}

func TestMiddleware_MissingTokenReturns401(t *testing.T) {
	store := newStubSessionStore()
	mw := Middleware(store)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })

	req := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	rec := httptest.NewRecorder()

	mw(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
	if called {
		t.Error("next handler must not be called on missing token")
	}
}

func TestMiddleware_InvalidTokenReturns401(t *testing.T) {
	store := newStubSessionStore()
	mw := Middleware(store)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })

	req := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	req.Header.Set("Authorization", "Bearer bogustoken")
	rec := httptest.NewRecorder()

	mw(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
	if called {
		t.Error("next handler must not be called on invalid token")
	}
}

func TestMiddleware_ValidBearerTokenAllowsRequest(t *testing.T) {
	store := newStubSessionStore()
	sess := validSession()
	_ = store.Create(context.Background(), "validtoken", sess)

	mw := Middleware(store)

	var capturedSession *models.Session
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedSession = SessionFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	req.Header.Set("Authorization", "Bearer validtoken")
	rec := httptest.NewRecorder()

	mw(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if capturedSession == nil {
		t.Fatal("expected session in context, got nil")
	}
	if capturedSession.UserID != sess.UserID {
		t.Errorf("session UserID mismatch: got %v, want %v", capturedSession.UserID, sess.UserID)
	}
}

func TestMiddleware_ValidCookieAllowsRequest(t *testing.T) {
	store := newStubSessionStore()
	sess := validSession()
	_ = store.Create(context.Background(), "cookietoken", sess)

	mw := Middleware(store)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "cookietoken"})
	rec := httptest.NewRecorder()

	mw(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

// --- RequireRole ---

func TestRequireRole_SufficientRoleAllowsRequest(t *testing.T) {
	store := newStubSessionStore()
	sess := &models.Session{UserID: uuid.New(), Role: models.RoleAdmin, CreatedAt: time.Now()}
	_ = store.Create(context.Background(), "admintoken", sess)

	authMw := Middleware(store)
	roleMw := RequireRole(models.RoleAdmin)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/admin", nil)
	req.Header.Set("Authorization", "Bearer admintoken")
	rec := httptest.NewRecorder()

	authMw(roleMw(next)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if !called {
		t.Error("next handler should have been called")
	}
}

func TestRequireRole_InsufficientRoleReturns403(t *testing.T) {
	store := newStubSessionStore()
	sess := &models.Session{UserID: uuid.New(), Role: models.RoleUser, CreatedAt: time.Now()}
	_ = store.Create(context.Background(), "usertoken", sess)

	authMw := Middleware(store)
	roleMw := RequireRole(models.RoleAdmin)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/admin", nil)
	req.Header.Set("Authorization", "Bearer usertoken")
	rec := httptest.NewRecorder()

	authMw(roleMw(next)).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
	if called {
		t.Error("next handler must not be called on insufficient role")
	}
}

func TestRequireRole_UserRoleAllowsUserEndpoint(t *testing.T) {
	store := newStubSessionStore()
	sess := &models.Session{UserID: uuid.New(), Role: models.RoleUser, CreatedAt: time.Now()}
	_ = store.Create(context.Background(), "usertoken2", sess)

	authMw := Middleware(store)
	roleMw := RequireRole(models.RoleUser)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	req.Header.Set("Authorization", "Bearer usertoken2")
	rec := httptest.NewRecorder()

	authMw(roleMw(next)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

// --- MustChangePassword enforcement (SEC-001) ---

// TestMiddleware_MustChangePasswordBlocks403 verifies that a session with
// MustChangePassword=true is blocked with 403 on any non-exempt endpoint.
func TestMiddleware_MustChangePasswordBlocks403(t *testing.T) {
	store := newStubSessionStore()
	sess := &models.Session{
		UserID:             uuid.New(),
		Role:               models.RoleUser,
		CreatedAt:          time.Now(),
		MustChangePassword: true,
	}
	_ = store.Create(context.Background(), "flaggedtoken", sess)

	mw := Middleware(store)
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/workers", nil)
	req.Header.Set("Authorization", "Bearer flaggedtoken")
	rec := httptest.NewRecorder()

	mw(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for MustChangePassword session on /api/workers, got %d", rec.Code)
	}
	if called {
		t.Error("next handler must not be called when MustChangePassword is true")
	}
}

// TestMiddleware_MustChangePasswordAllowsChangePasswordEndpoint verifies the
// change-password endpoint is exempt from the MustChangePassword block (AC-2).
func TestMiddleware_MustChangePasswordAllowsChangePasswordEndpoint(t *testing.T) {
	store := newStubSessionStore()
	sess := &models.Session{
		UserID:             uuid.New(),
		Role:               models.RoleUser,
		CreatedAt:          time.Now(),
		MustChangePassword: true,
	}
	_ = store.Create(context.Background(), "flaggedtoken2", sess)

	mw := Middleware(store)
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/auth/change-password", nil)
	req.Header.Set("Authorization", "Bearer flaggedtoken2")
	rec := httptest.NewRecorder()

	mw(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for change-password endpoint, got %d", rec.Code)
	}
	if !called {
		t.Error("next handler must be called for the change-password endpoint")
	}
}

// TestMiddleware_MustChangePasswordAllowsLogout verifies that logout is exempt
// so users can log out during the forced-change flow.
func TestMiddleware_MustChangePasswordAllowsLogout(t *testing.T) {
	store := newStubSessionStore()
	sess := &models.Session{
		UserID:             uuid.New(),
		Role:               models.RoleUser,
		CreatedAt:          time.Now(),
		MustChangePassword: true,
	}
	_ = store.Create(context.Background(), "flaggedtoken3", sess)

	mw := Middleware(store)
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer flaggedtoken3")
	rec := httptest.NewRecorder()

	mw(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204 for logout endpoint, got %d", rec.Code)
	}
	if !called {
		t.Error("next handler must be called for the logout endpoint")
	}
}

// TestMiddleware_MustChangeFalseAllowsNormalAccess verifies that a session without
// MustChangePassword set passes through to all protected routes normally.
func TestMiddleware_MustChangeFalseAllowsNormalAccess(t *testing.T) {
	store := newStubSessionStore()
	sess := &models.Session{
		UserID:             uuid.New(),
		Role:               models.RoleUser,
		CreatedAt:          time.Now(),
		MustChangePassword: false,
	}
	_ = store.Create(context.Background(), "normaltoken", sess)

	mw := Middleware(store)
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/workers", nil)
	req.Header.Set("Authorization", "Bearer normaltoken")
	rec := httptest.NewRecorder()

	mw(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for normal session, got %d", rec.Code)
	}
	if !called {
		t.Error("next handler must be called for normal session")
	}
}

// --- SessionFromContext ---

func TestSessionFromContext_ReturnsNilWhenNoSession(t *testing.T) {
	sess := SessionFromContext(context.Background())
	if sess != nil {
		t.Errorf("expected nil from empty context, got %v", sess)
	}
}

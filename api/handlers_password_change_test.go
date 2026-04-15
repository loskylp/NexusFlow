// Package api — unit tests for PasswordChangeHandler (SEC-001).
// Uses in-memory stubs from handlers_auth_test.go (same package).
// See: SEC-001, ADR-006
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/auth"
	"github.com/nxlabs/nexusflow/internal/models"
)

// --- stub helpers for password-change tests ---

// stubUserRepoWithPasswordChange extends stubUserRepo to record ChangePassword calls.
type stubUserRepoWithPasswordChange struct {
	*stubUserRepo
	changedIDs []uuid.UUID // IDs for which ChangePassword was called
}

func newStubUserRepoPC() *stubUserRepoWithPasswordChange {
	return &stubUserRepoWithPasswordChange{stubUserRepo: newStubUserRepo()}
}

// ChangePassword records the call and updates the in-memory user hash.
// This override satisfies db.UserRepository so the handler under test can call it.
func (r *stubUserRepoWithPasswordChange) ChangePassword(_ context.Context, id uuid.UUID, passwordHash string) error {
	r.changedIDs = append(r.changedIDs, id)
	for _, u := range r.users {
		if u.ID == id {
			u.PasswordHash = passwordHash
			u.MustChangePassword = false
		}
	}
	return nil
}

// addMustChangeUser adds a user that has MustChangePassword=true.
func addMustChangeUser(repo *stubUserRepoWithPasswordChange, t *testing.T, username, password string, role models.Role) *models.User {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("addMustChangeUser: hash: %v", err)
	}
	u := &models.User{
		ID:                 uuid.New(),
		Username:           username,
		PasswordHash:       hash,
		Role:               role,
		Active:             true,
		MustChangePassword: true,
		CreatedAt:          time.Now(),
	}
	repo.addUser(u)
	return u
}

// sessionWithUser stores a session for the given user in the store and returns the token.
func sessionWithUser(t *testing.T, sessions *stubSessionStore, user *models.User) string {
	t.Helper()
	token, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("sessionWithUser: generate token: %v", err)
	}
	sess := &models.Session{
		UserID:             user.ID,
		Role:               user.Role,
		CreatedAt:          time.Now(),
		MustChangePassword: user.MustChangePassword,
	}
	if err := sessions.Create(context.Background(), token, sess); err != nil {
		t.Fatalf("sessionWithUser: create session: %v", err)
	}
	return token
}

// changePasswordRequest builds a POST /api/auth/change-password request with the session injected.
func changePasswordReq(currentPassword, newPassword string, sess *models.Session) *http.Request {
	body, _ := json.Marshal(map[string]string{
		"currentPassword": currentPassword,
		"newPassword":     newPassword,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/change-password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// Inject session into context as the auth middleware would.
	ctx := contextWithSession(req.Context(), sess)
	return req.WithContext(ctx)
}

// contextWithSession injects a *models.Session into a context by running the auth
// middleware with a temporary stub store. The session is stored as a regular (non-flagged)
// session so the middleware always calls the inner handler regardless of MustChangePassword —
// the ChangePassword handler itself handles that flag appropriately.
//
// This helper simulates what the real auth middleware does when it validates a token and
// stores the session in the request context via context.WithValue.
func contextWithSession(ctx context.Context, sess *models.Session) context.Context {
	// Use a copy of the session with MustChangePassword=false for context injection.
	// The handler under test receives the real session via the changePasswordReq function.
	// We need a clean session here so the middleware's MustChangePassword guard
	// does not block the inner handler from being called.
	injectSess := &models.Session{
		UserID:             sess.UserID,
		Role:               sess.Role,
		CreatedAt:          sess.CreatedAt,
		MustChangePassword: false, // allow the middleware to pass through to the inner handler
	}

	store := newStubSessionStore()
	token := "test-inject-token"
	_ = store.Create(ctx, token, injectSess)

	var injectedCtx context.Context
	handler := auth.Middleware(store)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		injectedCtx = r.Context()
	}))
	rec := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/api/auth/change-password", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	handler.ServeHTTP(rec, req)
	return injectedCtx
}

// --- ChangePassword tests ---

// TestChangePassword_CorrectPasswordReturns204 verifies the happy path:
// valid current password + valid new password → 204, password updated, flag cleared.
func TestChangePassword_CorrectPasswordReturns204(t *testing.T) {
	users := newStubUserRepoPC()
	sessions := newStubSessionStore()
	u := addMustChangeUser(users, t, "alice", "oldpass", models.RoleAdmin)
	sessionToken := sessionWithUser(t, sessions, u)

	sess := &models.Session{
		UserID:             u.ID,
		Role:               u.Role,
		CreatedAt:          time.Now(),
		MustChangePassword: true,
	}
	_ = sessions.Create(context.Background(), sessionToken, sess)

	srv := &Server{
		cfg:      testServer(users.stubUserRepo, sessions).cfg,
		users:    users,
		sessions: sessions,
	}
	h := &PasswordChangeHandler{server: srv}

	req := changePasswordReq("oldpass", "newpassword8", sess)
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	rec := httptest.NewRecorder()

	h.ChangePassword(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(users.changedIDs) != 1 || users.changedIDs[0] != u.ID {
		t.Error("expected ChangePassword to be called for the user")
	}
}

// TestChangePassword_WrongCurrentPasswordReturns401 verifies AC-3:
// incorrect current password → 401.
func TestChangePassword_WrongCurrentPasswordReturns401(t *testing.T) {
	users := newStubUserRepoPC()
	sessions := newStubSessionStore()
	u := addMustChangeUser(users, t, "bob", "correctpass", models.RoleUser)

	sess := &models.Session{
		UserID:             u.ID,
		Role:               u.Role,
		CreatedAt:          time.Now(),
		MustChangePassword: true,
	}

	srv := &Server{
		cfg:      testServer(users.stubUserRepo, sessions).cfg,
		users:    users,
		sessions: sessions,
	}
	h := &PasswordChangeHandler{server: srv}

	req := changePasswordReq("wrongpass", "newpassword8", sess)
	rec := httptest.NewRecorder()

	h.ChangePassword(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(users.changedIDs) != 0 {
		t.Error("ChangePassword must not be called when current password is wrong")
	}
}

// TestChangePassword_NewPasswordTooShortReturns400 verifies AC-4:
// new password shorter than 8 characters → 400.
func TestChangePassword_NewPasswordTooShortReturns400(t *testing.T) {
	users := newStubUserRepoPC()
	sessions := newStubSessionStore()
	u := addMustChangeUser(users, t, "carol", "correctpass", models.RoleUser)

	sess := &models.Session{
		UserID:             u.ID,
		Role:               u.Role,
		CreatedAt:          time.Now(),
		MustChangePassword: true,
	}

	srv := &Server{
		cfg:      testServer(users.stubUserRepo, sessions).cfg,
		users:    users,
		sessions: sessions,
	}
	h := &PasswordChangeHandler{server: srv}

	// 7 characters — one short of the 8-char minimum.
	req := changePasswordReq("correctpass", "short7!", sess)
	rec := httptest.NewRecorder()

	h.ChangePassword(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(users.changedIDs) != 0 {
		t.Error("ChangePassword must not be called when new password is too short")
	}
}

// TestChangePassword_ExactlyEightCharsNewPasswordAccepted verifies the boundary:
// new password of exactly 8 characters is accepted.
func TestChangePassword_ExactlyEightCharsNewPasswordAccepted(t *testing.T) {
	users := newStubUserRepoPC()
	sessions := newStubSessionStore()
	u := addMustChangeUser(users, t, "dave", "correctpass", models.RoleUser)

	sess := &models.Session{
		UserID:             u.ID,
		Role:               u.Role,
		CreatedAt:          time.Now(),
		MustChangePassword: true,
	}

	srv := &Server{
		cfg:      testServer(users.stubUserRepo, sessions).cfg,
		users:    users,
		sessions: sessions,
	}
	h := &PasswordChangeHandler{server: srv}

	req := changePasswordReq("correctpass", "exactly8", sess) // exactly 8 chars
	rec := httptest.NewRecorder()

	h.ChangePassword(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204 for 8-char password, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestChangePassword_AllSessionsInvalidatedAfterChange verifies AC-6:
// after a successful password change, all sessions for the user are invalidated.
func TestChangePassword_AllSessionsInvalidatedAfterChange(t *testing.T) {
	users := newStubUserRepoPC()
	sessions := newStubSessionStore()
	u := addMustChangeUser(users, t, "eve", "oldpass", models.RoleUser)

	// Create two sessions for the same user to verify both are invalidated.
	token1 := sessionWithUser(t, sessions, u)
	token2 := sessionWithUser(t, sessions, u)

	sess := &models.Session{
		UserID:             u.ID,
		Role:               u.Role,
		CreatedAt:          time.Now(),
		MustChangePassword: true,
	}

	srv := &Server{
		cfg:      testServer(users.stubUserRepo, sessions).cfg,
		users:    users,
		sessions: sessions,
	}
	h := &PasswordChangeHandler{server: srv}

	req := changePasswordReq("oldpass", "newpassword8", sess)
	req.Header.Set("Authorization", "Bearer "+token1)
	rec := httptest.NewRecorder()

	h.ChangePassword(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	// Both sessions must be gone.
	s1, _ := sessions.Get(context.Background(), token1)
	if s1 != nil {
		t.Error("token1 (used for the request) should be invalidated after password change")
	}
	s2, _ := sessions.Get(context.Background(), token2)
	if s2 != nil {
		t.Error("token2 (other session for same user) should be invalidated after password change")
	}
}

// TestChangePassword_MissingSessionReturns403 verifies that an unauthenticated request
// (no session in context) is rejected with 403.
func TestChangePassword_MissingSessionReturns403(t *testing.T) {
	users := newStubUserRepoPC()
	sessions := newStubSessionStore()

	srv := &Server{
		cfg:      testServer(users.stubUserRepo, sessions).cfg,
		users:    users,
		sessions: sessions,
	}
	h := &PasswordChangeHandler{server: srv}

	// No session injected into context.
	body, _ := json.Marshal(map[string]string{
		"currentPassword": "anything",
		"newPassword":     "newpassword8",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/change-password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.ChangePassword(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for unauthenticated request, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestChangePassword_MalformedBodyReturns400 verifies that non-JSON bodies are rejected.
func TestChangePassword_MalformedBodyReturns400(t *testing.T) {
	users := newStubUserRepoPC()
	sessions := newStubSessionStore()
	u := addMustChangeUser(users, t, "frank", "pass", models.RoleUser)

	sess := &models.Session{
		UserID:    u.ID,
		Role:      u.Role,
		CreatedAt: time.Now(),
	}

	srv := &Server{
		cfg:      testServer(users.stubUserRepo, sessions).cfg,
		users:    users,
		sessions: sessions,
	}
	h := &PasswordChangeHandler{server: srv}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/change-password", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithSession(req.Context(), sess)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.ChangePassword(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for malformed body, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestChangePassword_EmptyFieldsReturns400 verifies that missing required fields are rejected.
func TestChangePassword_EmptyFieldsReturns400(t *testing.T) {
	users := newStubUserRepoPC()
	sessions := newStubSessionStore()
	u := addMustChangeUser(users, t, "grace", "pass", models.RoleUser)

	sess := &models.Session{
		UserID:    u.ID,
		Role:      u.Role,
		CreatedAt: time.Now(),
	}

	srv := &Server{
		cfg:      testServer(users.stubUserRepo, sessions).cfg,
		users:    users,
		sessions: sessions,
	}
	h := &PasswordChangeHandler{server: srv}

	body, _ := json.Marshal(map[string]string{
		"currentPassword": "",
		"newPassword":     "",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/change-password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithSession(req.Context(), sess)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.ChangePassword(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty fields, got %d: %s", rec.Code, rec.Body.String())
	}
}

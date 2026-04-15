// Package api — unit tests for AuthHandler (Login, Logout).
// Uses in-memory stubs for UserRepository and SessionStore to avoid external dependencies.
// See: ADR-006, TASK-003
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
	"github.com/nxlabs/nexusflow/internal/config"
	"github.com/nxlabs/nexusflow/internal/db"
	"github.com/nxlabs/nexusflow/internal/models"
)

// --- stubs ---

// stubUserRepo is an in-memory UserRepository for testing.
type stubUserRepo struct {
	users map[string]*models.User // keyed by username
}

func newStubUserRepo() *stubUserRepo {
	return &stubUserRepo{users: make(map[string]*models.User)}
}

func (r *stubUserRepo) addUser(u *models.User) {
	r.users[u.Username] = u
}

func (r *stubUserRepo) Create(_ context.Context, u *models.User) (*models.User, error) {
	if _, exists := r.users[u.Username]; exists {
		return nil, db.ErrConflict
	}
	r.users[u.Username] = u
	return u, nil
}

func (r *stubUserRepo) GetByID(_ context.Context, id uuid.UUID) (*models.User, error) {
	for _, u := range r.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, nil
}

func (r *stubUserRepo) GetByUsername(_ context.Context, username string) (*models.User, error) {
	return r.users[username], nil
}

func (r *stubUserRepo) List(_ context.Context) ([]*models.User, error) {
	out := make([]*models.User, 0, len(r.users))
	for _, u := range r.users {
		out = append(out, u)
	}
	return out, nil
}

func (r *stubUserRepo) Deactivate(_ context.Context, id uuid.UUID) error {
	for _, u := range r.users {
		if u.ID == id {
			u.Active = false
			return nil
		}
	}
	return nil
}

// ChangePassword is a no-op stub satisfying db.UserRepository for tests that do not
// exercise password change behaviour. The real implementation belongs to SEC-001.
func (r *stubUserRepo) ChangePassword(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}

// stubSessionStore is an in-memory SessionStore for testing.
type stubSessionStore struct {
	sessions map[string]*models.Session
}

func newStubSessionStore() *stubSessionStore {
	return &stubSessionStore{sessions: make(map[string]*models.Session)}
}

func (s *stubSessionStore) Create(_ context.Context, token string, sess *models.Session) error {
	s.sessions[token] = sess
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

// --- test helpers ---

// testServer constructs a minimal Server with stub user and session stores.
func testServer(users *stubUserRepo, sessions *stubSessionStore) *Server {
	return &Server{
		cfg: &config.Config{
			Env: "development",
		},
		users:    users,
		sessions: sessions,
	}
}

// activeUserWithPassword creates a models.User with a bcrypt-hashed password, active=true.
func activeUserWithPassword(t *testing.T, username, password string, role models.Role) *models.User {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("activeUserWithPassword: hash: %v", err)
	}
	return &models.User{
		ID:           uuid.New(),
		Username:     username,
		PasswordHash: hash,
		Role:         role,
		Active:       true,
		CreatedAt:    time.Now(),
	}
}

func postLoginRequest(username, password string) *http.Request {
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// --- Login tests ---

func TestLogin_ValidCredentialsReturns200WithTokenAndCookie(t *testing.T) {
	users := newStubUserRepo()
	sessions := newStubSessionStore()
	u := activeUserWithPassword(t, "alice", "secret", models.RoleUser)
	users.addUser(u)

	srv := testServer(users, sessions)
	h := &AuthHandler{server: srv}

	rec := httptest.NewRecorder()
	h.Login(rec, postLoginRequest("alice", "secret"))

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp loginResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Token == "" {
		t.Error("expected non-empty token in response body")
	}
	if resp.User.Username != "alice" {
		t.Errorf("expected username=alice, got %q", resp.User.Username)
	}

	// Cookie must be set.
	cookies := rec.Result().Cookies()
	var found bool
	for _, c := range cookies {
		if c.Name == "session" {
			found = true
			if c.Value != resp.Token {
				t.Errorf("cookie value %q != body token %q", c.Value, resp.Token)
			}
			if !c.HttpOnly {
				t.Error("session cookie must be HttpOnly")
			}
		}
	}
	if !found {
		t.Error("expected session cookie in response")
	}

	// Session must exist in the store.
	sess, _ := sessions.Get(context.Background(), resp.Token)
	if sess == nil {
		t.Error("session not found in store after login")
	}
}

func TestLogin_InvalidPasswordReturns401(t *testing.T) {
	users := newStubUserRepo()
	u := activeUserWithPassword(t, "bob", "correct", models.RoleUser)
	users.addUser(u)

	srv := testServer(users, newStubSessionStore())
	h := &AuthHandler{server: srv}

	rec := httptest.NewRecorder()
	h.Login(rec, postLoginRequest("bob", "wrong"))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestLogin_UnknownUsernameReturns401(t *testing.T) {
	srv := testServer(newStubUserRepo(), newStubSessionStore())
	h := &AuthHandler{server: srv}

	rec := httptest.NewRecorder()
	h.Login(rec, postLoginRequest("nobody", "anything"))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestLogin_InactiveUserReturns401(t *testing.T) {
	users := newStubUserRepo()
	u := activeUserWithPassword(t, "carol", "pass", models.RoleUser)
	u.Active = false
	users.addUser(u)

	srv := testServer(users, newStubSessionStore())
	h := &AuthHandler{server: srv}

	rec := httptest.NewRecorder()
	h.Login(rec, postLoginRequest("carol", "pass"))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for inactive user, got %d", rec.Code)
	}
}

func TestLogin_MalformedBodyReturns400(t *testing.T) {
	srv := testServer(newStubUserRepo(), newStubSessionStore())
	h := &AuthHandler{server: srv}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Login(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestLogin_EmptyUsernameReturns400(t *testing.T) {
	srv := testServer(newStubUserRepo(), newStubSessionStore())
	h := &AuthHandler{server: srv}

	rec := httptest.NewRecorder()
	h.Login(rec, postLoginRequest("", "pass"))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty username, got %d", rec.Code)
	}
}

// --- Logout tests ---

func TestLogout_WithValidBearerTokenDeletesSessionReturns204(t *testing.T) {
	sessions := newStubSessionStore()
	sess := &models.Session{UserID: uuid.New(), Role: models.RoleUser, CreatedAt: time.Now()}
	_ = sessions.Create(context.Background(), "mytoken", sess)

	srv := testServer(newStubUserRepo(), sessions)
	h := &AuthHandler{server: srv}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer mytoken")
	rec := httptest.NewRecorder()

	h.Logout(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}

	// Session must be deleted.
	got, _ := sessions.Get(context.Background(), "mytoken")
	if got != nil {
		t.Error("session should be deleted after logout")
	}
}

func TestLogout_WithCookieDeletesSessionReturns204(t *testing.T) {
	sessions := newStubSessionStore()
	sess := &models.Session{UserID: uuid.New(), Role: models.RoleUser, CreatedAt: time.Now()}
	_ = sessions.Create(context.Background(), "cookietoken", sess)

	srv := testServer(newStubUserRepo(), sessions)
	h := &AuthHandler{server: srv}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "cookietoken"})
	rec := httptest.NewRecorder()

	h.Logout(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	got, _ := sessions.Get(context.Background(), "cookietoken")
	if got != nil {
		t.Error("session should be deleted after logout")
	}
}

func TestLogout_NoTokenReturns401(t *testing.T) {
	srv := testServer(newStubUserRepo(), newStubSessionStore())
	h := &AuthHandler{server: srv}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	rec := httptest.NewRecorder()

	h.Logout(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

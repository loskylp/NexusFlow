// Package api — unit tests for UserHandler (CreateUser, ListUsers, DeactivateUser).
// Uses in-memory stubs defined in handlers_auth_test.go.
// See: REQ-020, TASK-017
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/auth"
	"github.com/nxlabs/nexusflow/internal/models"
)

// injectSession wraps req with the session injected by auth.Middleware,
// using the same mechanism that the production middleware stack uses.
// Returns the captured request with the session in its context.
func injectSession(t *testing.T, req *http.Request, sess *models.Session, store *stubSessionStore) *http.Request {
	t.Helper()
	token := "test-token-" + uuid.New().String()
	_ = store.Create(context.Background(), token, sess)
	req.Header.Set("Authorization", "Bearer "+token)

	var captured *http.Request
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		captured = r
	})
	mw := auth.Middleware(store)
	mw(inner).ServeHTTP(httptest.NewRecorder(), req)
	if captured == nil {
		t.Fatal("auth.Middleware did not call inner handler — session injection failed")
	}
	return captured
}

// adminRequest builds an HTTP request carrying an admin session in its context.
func adminRequest(t *testing.T, method, path string, body *bytes.Reader) *http.Request {
	t.Helper()
	if body == nil {
		body = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", "application/json")
	store := newStubSessionStore()
	sess := &models.Session{UserID: uuid.New(), Role: models.RoleAdmin, CreatedAt: time.Now()}
	return injectSession(t, req, sess, store)
}

// jsonBody encodes v as JSON and returns a *bytes.Reader for use as an HTTP body.
func jsonBody(v any) *bytes.Reader {
	b, _ := json.Marshal(v)
	return bytes.NewReader(b)
}

// --- POST /api/users tests ---

func TestCreateUser_AdminCreatesUserReturns201(t *testing.T) {
	users := newStubUserRepo()
	sessions := newStubSessionStore()
	srv := &Server{users: users, sessions: sessions}
	h := &UserHandler{server: srv}

	req := adminRequest(t, http.MethodPost, "/api/users", jsonBody(map[string]string{
		"username": "newuser",
		"password": "secret123",
		"role":     "user",
	}))
	rec := httptest.NewRecorder()
	h.CreateUser(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp userResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Username != "newuser" {
		t.Errorf("expected username=newuser, got %q", resp.Username)
	}
	if resp.Role != models.RoleUser {
		t.Errorf("expected role=user, got %q", resp.Role)
	}
	if !resp.Active {
		t.Error("expected active=true for newly created user")
	}
	if resp.ID == uuid.Nil {
		t.Error("expected non-zero ID in response")
	}
}

func TestCreateUser_PasswordNotInResponse(t *testing.T) {
	users := newStubUserRepo()
	srv := &Server{users: users, sessions: newStubSessionStore()}
	h := &UserHandler{server: srv}

	req := adminRequest(t, http.MethodPost, "/api/users", jsonBody(map[string]string{
		"username": "alice",
		"password": "hunter2",
		"role":     "user",
	}))
	rec := httptest.NewRecorder()
	h.CreateUser(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}

	// Ensure password-related fields are absent from the raw JSON body.
	var raw map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&raw)
	for _, key := range []string{"password", "passwordHash", "password_hash"} {
		if _, ok := raw[key]; ok {
			t.Errorf("response must not contain field %q", key)
		}
	}
}

func TestCreateUser_MalformedBodyReturns400(t *testing.T) {
	srv := &Server{users: newStubUserRepo(), sessions: newStubSessionStore()}
	h := &UserHandler{server: srv}

	req := adminRequest(t, http.MethodPost, "/api/users", bytes.NewReader([]byte("not json")))
	rec := httptest.NewRecorder()
	h.CreateUser(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestCreateUser_MissingUsernameReturns400(t *testing.T) {
	srv := &Server{users: newStubUserRepo(), sessions: newStubSessionStore()}
	h := &UserHandler{server: srv}

	req := adminRequest(t, http.MethodPost, "/api/users", jsonBody(map[string]string{
		"password": "pass",
		"role":     "user",
	}))
	rec := httptest.NewRecorder()
	h.CreateUser(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestCreateUser_MissingPasswordReturns400(t *testing.T) {
	srv := &Server{users: newStubUserRepo(), sessions: newStubSessionStore()}
	h := &UserHandler{server: srv}

	req := adminRequest(t, http.MethodPost, "/api/users", jsonBody(map[string]string{
		"username": "bob",
		"role":     "user",
	}))
	rec := httptest.NewRecorder()
	h.CreateUser(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestCreateUser_InvalidRoleReturns400(t *testing.T) {
	srv := &Server{users: newStubUserRepo(), sessions: newStubSessionStore()}
	h := &UserHandler{server: srv}

	req := adminRequest(t, http.MethodPost, "/api/users", jsonBody(map[string]string{
		"username": "bob",
		"password": "pass",
		"role":     "superadmin",
	}))
	rec := httptest.NewRecorder()
	h.CreateUser(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid role, got %d", rec.Code)
	}
}

func TestCreateUser_DuplicateUsernameReturns409(t *testing.T) {
	users := newStubUserRepo()
	hash, _ := auth.HashPassword("pass")
	users.addUser(&models.User{
		ID:           uuid.New(),
		Username:     "taken",
		PasswordHash: hash,
		Role:         models.RoleUser,
		Active:       true,
		CreatedAt:    time.Now(),
	})

	srv := &Server{users: users, sessions: newStubSessionStore()}
	h := &UserHandler{server: srv}

	req := adminRequest(t, http.MethodPost, "/api/users", jsonBody(map[string]string{
		"username": "taken",
		"password": "pass2",
		"role":     "user",
	}))
	rec := httptest.NewRecorder()
	h.CreateUser(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409 for duplicate username, got %d", rec.Code)
	}
}

// --- GET /api/users tests ---

func TestListUsers_AdminReceivesAllUsersReturns200(t *testing.T) {
	users := newStubUserRepo()
	hash, _ := auth.HashPassword("pass")
	users.addUser(&models.User{ID: uuid.New(), Username: "alice", PasswordHash: hash, Role: models.RoleUser, Active: true, CreatedAt: time.Now()})
	users.addUser(&models.User{ID: uuid.New(), Username: "bob", PasswordHash: hash, Role: models.RoleAdmin, Active: false, CreatedAt: time.Now()})

	srv := &Server{users: users, sessions: newStubSessionStore()}
	h := &UserHandler{server: srv}

	req := adminRequest(t, http.MethodGet, "/api/users", nil)
	rec := httptest.NewRecorder()
	h.ListUsers(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp []userResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp) != 2 {
		t.Errorf("expected 2 users, got %d", len(resp))
	}
}

func TestListUsers_ResponseExcludesPasswordHash(t *testing.T) {
	users := newStubUserRepo()
	hash, _ := auth.HashPassword("pass")
	users.addUser(&models.User{ID: uuid.New(), Username: "alice", PasswordHash: hash, Role: models.RoleUser, Active: true, CreatedAt: time.Now()})

	srv := &Server{users: users, sessions: newStubSessionStore()}
	h := &UserHandler{server: srv}

	req := adminRequest(t, http.MethodGet, "/api/users", nil)
	rec := httptest.NewRecorder()
	h.ListUsers(rec, req)

	var raw []map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&raw)
	if len(raw) == 0 {
		t.Fatal("expected at least one user")
	}
	for _, u := range raw {
		for _, key := range []string{"passwordHash", "password_hash"} {
			if _, ok := u[key]; ok {
				t.Errorf("response must not contain field %q", key)
			}
		}
	}
}

func TestListUsers_EmptyStoreReturnsEmptyArray(t *testing.T) {
	srv := &Server{users: newStubUserRepo(), sessions: newStubSessionStore()}
	h := &UserHandler{server: srv}

	req := adminRequest(t, http.MethodGet, "/api/users", nil)
	rec := httptest.NewRecorder()
	h.ListUsers(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp []userResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp == nil || len(resp) != 0 {
		t.Errorf("expected empty array, got %v", resp)
	}
}

// --- PUT /api/users/{id}/deactivate tests ---

func TestDeactivateUser_AdminDeactivatesExistingUserReturns204(t *testing.T) {
	users := newStubUserRepo()
	sessions := newStubSessionStore()

	targetID := uuid.New()
	hash, _ := auth.HashPassword("pass")
	users.addUser(&models.User{ID: targetID, Username: "victim", PasswordHash: hash, Role: models.RoleUser, Active: true, CreatedAt: time.Now()})

	// Pre-seed an active session for the target user so we can verify it's deleted.
	_ = sessions.Create(context.Background(), "victim-token", &models.Session{
		UserID:    targetID,
		Role:      models.RoleUser,
		CreatedAt: time.Now(),
	})

	srv := &Server{users: users, sessions: sessions}
	h := &UserHandler{server: srv}

	req := adminRequest(t, http.MethodPut, "/api/users/"+targetID.String()+"/deactivate", nil)
	// Inject chi URL param.
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", targetID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	h.DeactivateUser(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	// User must now be inactive.
	u, _ := users.GetByID(context.Background(), targetID)
	if u == nil {
		t.Fatal("user not found after deactivation")
	}
	if u.Active {
		t.Error("expected user to be inactive after deactivation")
	}

	// The victim's session must be deleted.
	sess, _ := sessions.Get(context.Background(), "victim-token")
	if sess != nil {
		t.Error("expected victim session to be invalidated after deactivation")
	}
}

func TestDeactivateUser_NonExistentUserReturns404(t *testing.T) {
	srv := &Server{users: newStubUserRepo(), sessions: newStubSessionStore()}
	h := &UserHandler{server: srv}

	missingID := uuid.New()
	req := adminRequest(t, http.MethodPut, "/api/users/"+missingID.String()+"/deactivate", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", missingID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	h.DeactivateUser(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestDeactivateUser_InvalidIDReturns400(t *testing.T) {
	srv := &Server{users: newStubUserRepo(), sessions: newStubSessionStore()}
	h := &UserHandler{server: srv}

	req := adminRequest(t, http.MethodPut, "/api/users/not-a-uuid/deactivate", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "not-a-uuid")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	h.DeactivateUser(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid UUID, got %d", rec.Code)
	}
}

// --- Role enforcement via RequireRole middleware (integration with chi router) ---

func TestUserEndpoints_NonAdminReceives403(t *testing.T) {
	users := newStubUserRepo()
	sessions := newStubSessionStore()

	// Create a non-admin session token.
	userID := uuid.New()
	token := "user-token"
	_ = sessions.Create(context.Background(), token, &models.Session{
		UserID:    userID,
		Role:      models.RoleUser,
		CreatedAt: time.Now(),
	})

	srv := &Server{users: users, sessions: sessions}
	handler := buildUserRoutes(srv)

	endpoints := []struct {
		method string
		path   string
		body   *bytes.Reader
	}{
		{http.MethodPost, "/api/users", jsonBody(map[string]string{"username": "x", "password": "y", "role": "user"})},
		{http.MethodGet, "/api/users", bytes.NewReader(nil)},
		{http.MethodPut, "/api/users/" + uuid.New().String() + "/deactivate", bytes.NewReader(nil)},
	}

	for _, ep := range endpoints {
		req := httptest.NewRequest(ep.method, ep.path, ep.body)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s: expected 403 for non-admin, got %d", ep.method, ep.path, rec.Code)
		}
	}
}

func TestUserEndpoints_UnauthenticatedReceives401(t *testing.T) {
	srv := &Server{users: newStubUserRepo(), sessions: newStubSessionStore()}
	handler := buildUserRoutes(srv)

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for unauthenticated request, got %d", rec.Code)
	}
}

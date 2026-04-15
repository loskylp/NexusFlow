// Package api — Auth handlers: login and logout.
// POST /api/auth/login   — issues a session token as both an HTTP-only cookie and a Bearer token response body.
// POST /api/auth/logout  — invalidates the session.
// See: ADR-006, TASK-003
package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/auth"
	"github.com/nxlabs/nexusflow/internal/models"
)

// AuthHandler handles authentication endpoints.
// Depends on UserRepository (credential lookup) and SessionStore (session lifecycle).
// See: ADR-006, TASK-003
type AuthHandler struct {
	server *Server
}

// loginRequest is the JSON body for POST /api/auth/login.
type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// loginResponse is the JSON body returned on successful login.
type loginResponse struct {
	Token string     `json:"token"`
	User  publicUser `json:"user"`
}

// publicUser is the user representation safe to return in API responses
// (omits PasswordHash per models.User json:"-" tag).
// SEC-001: mustChangePassword is included so the frontend can redirect to
// the change-password page immediately after login when the flag is set.
type publicUser struct {
	ID                 uuid.UUID   `json:"id"`
	Username           string      `json:"username"`
	Role               models.Role `json:"role"`
	MustChangePassword bool        `json:"mustChangePassword"`
}

// Login handles POST /api/auth/login.
// Validates username/password against bcrypt hash (ADR-006).
// On success: generates a 256-bit session token, stores it in Redis, returns it
// in an HTTP-only Secure cookie and in the response body as { "token": "..." }.
//
// Request body:
//
//	{ "username": "string", "password": "string" }
//
// Responses:
//
//	200 OK:           { "token": "...", "user": { id, username, role } }
//	400 Bad Request:  malformed JSON body
//	401 Unauthorized: username not found or password does not match
//	500 Internal:     Redis or database failure
//
// Preconditions:
//   - Request body is valid JSON with non-empty username and password.
//
// Postconditions:
//   - On 200: session exists in Redis with 24h TTL; cookie is set on the response.
//   - On 401: no session is created; no cookie is set.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if h.server.users == nil || h.server.sessions == nil {
		http.Error(w, "auth not configured", http.StatusInternalServerError)
		return
	}

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		http.Error(w, "username and password are required", http.StatusBadRequest)
		return
	}

	user, err := h.server.users.GetByUsername(r.Context(), req.Username)
	if err != nil {
		log.Printf("auth.Login: GetByUsername(%q): %v", req.Username, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if user == nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if !user.Active {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	if err := auth.VerifyPassword(req.Password, user.PasswordHash); err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	token, err := auth.GenerateToken()
	if err != nil {
		log.Printf("auth.Login: GenerateToken: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// SEC-001: carry the MustChangePassword flag into the session so the auth
	// middleware can enforce the mandatory first-login password rotation.
	sess := &models.Session{
		UserID:             user.ID,
		Role:               user.Role,
		CreatedAt:          time.Now().UTC(),
		MustChangePassword: user.MustChangePassword,
	}
	if err := h.server.sessions.Create(r.Context(), token, sess); err != nil {
		log.Printf("auth.Login: sessions.Create: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Set HTTP-only secure cookie for web GUI (XSS-resistant per ADR-006).
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.server.cfg.Env != "development",
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int((24 * time.Hour).Seconds()),
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(loginResponse{
		Token: token,
		User: publicUser{
			ID:                 user.ID,
			Username:           user.Username,
			Role:               user.Role,
			MustChangePassword: user.MustChangePassword,
		},
	})
}

// Logout handles POST /api/auth/logout.
// Reads the session token from cookie or Authorization header, deletes it from Redis.
// Always returns 204 — even if the session was already expired or not found.
//
// Responses:
//
//	204 No Content: session deleted (or was already absent)
//	401:            no token present in request (handled by auth middleware on protected routes)
//
// Postconditions:
//   - On 204: the session token is no longer valid for subsequent requests.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	sess := auth.SessionFromContext(r.Context())
	if sess == nil {
		// No session in context means the middleware rejected or this route is unprotected.
		// Extract token directly so Logout can still clear it.
		token := extractBearerOrCookie(r)
		if token == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if h.server.sessions != nil {
			_ = h.server.sessions.Delete(r.Context(), token)
		}
		clearSessionCookie(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Session was injected by middleware — extract token from request to delete it.
	token := extractBearerOrCookie(r)
	if token != "" && h.server.sessions != nil {
		if err := h.server.sessions.Delete(r.Context(), token); err != nil {
			log.Printf("auth.Logout: sessions.Delete: %v", err)
		}
	}

	clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// extractBearerOrCookie extracts the session token from the Authorization header or cookie.
// Returns an empty string if neither is present.
func extractBearerOrCookie(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	if cookie, err := r.Cookie("session"); err == nil {
		return cookie.Value
	}
	return ""
}

// clearSessionCookie instructs the browser to expire the session cookie immediately.
func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

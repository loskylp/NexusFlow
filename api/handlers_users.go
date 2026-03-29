// Package api — User management handlers (admin-only).
// POST /api/users        — create a new user account.
// GET  /api/users        — list all user accounts.
// PUT  /api/users/{id}/deactivate — deactivate an account and invalidate its sessions.
// All three endpoints require the caller to be authenticated and have role "admin".
// See: REQ-020, TASK-017
package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/auth"
	"github.com/nxlabs/nexusflow/internal/db"
	"github.com/nxlabs/nexusflow/internal/models"
)

// UserHandler handles user management REST endpoints.
// All handlers require admin role; enforcement is applied via RequireRole middleware
// at the router level (see buildUserRoutes and server.Handler).
// See: REQ-020, TASK-017
type UserHandler struct {
	server *Server
}

// createUserRequest is the JSON body for POST /api/users.
type createUserRequest struct {
	Username string      `json:"username"`
	Password string      `json:"password"`
	Role     models.Role `json:"role"`
}

// userResponse is the safe public representation of a User returned by the user
// management endpoints. The PasswordHash field is deliberately excluded.
// See: REQ-020
type userResponse struct {
	ID        uuid.UUID   `json:"id"`
	Username  string      `json:"username"`
	Role      models.Role `json:"role"`
	Active    bool        `json:"active"`
	CreatedAt time.Time   `json:"createdAt"`
}

// toUserResponse converts a domain models.User to a userResponse, omitting the password hash.
func toUserResponse(u *models.User) userResponse {
	return userResponse{
		ID:        u.ID,
		Username:  u.Username,
		Role:      u.Role,
		Active:    u.Active,
		CreatedAt: u.CreatedAt,
	}
}

// isValidRole returns true when r is one of the two recognised roles (admin, user).
func isValidRole(r models.Role) bool {
	return r == models.RoleAdmin || r == models.RoleUser
}

// CreateUser handles POST /api/users.
// Parses the JSON body, validates the fields, bcrypt-hashes the password at cost 12,
// and inserts the new user via UserRepository.
//
// Request body:
//
//	{ "username": "string", "password": "string", "role": "admin"|"user" }
//
// Responses:
//
//	201 Created:   { id, username, role, active, createdAt }
//	400 Bad Request: malformed JSON, empty username/password, or invalid role
//	409 Conflict:    username already taken
//	500 Internal:    database failure
//
// Preconditions:
//   - Caller is authenticated and has role "admin" (enforced by middleware).
//
// Postconditions:
//   - On 201: user is persisted with active=true and bcrypt-hashed password.
func (h *UserHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		http.Error(w, "username is required", http.StatusBadRequest)
		return
	}
	if req.Password == "" {
		http.Error(w, "password is required", http.StatusBadRequest)
		return
	}
	if !isValidRole(req.Role) {
		http.Error(w, "role must be 'admin' or 'user'", http.StatusBadRequest)
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		log.Printf("users.CreateUser: hash password: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	user := &models.User{
		ID:           uuid.New(),
		Username:     req.Username,
		PasswordHash: hash,
		Role:         req.Role,
		Active:       true,
		CreatedAt:    time.Now().UTC(),
	}

	created, err := h.server.users.Create(r.Context(), user)
	if err != nil {
		if err == db.ErrConflict {
			http.Error(w, "username already taken", http.StatusConflict)
			return
		}
		log.Printf("users.CreateUser: repository.Create: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(toUserResponse(created))
}

// ListUsers handles GET /api/users.
// Returns all user accounts ordered by creation time.
//
// Responses:
//
//	200 OK:       [ { id, username, role, active, createdAt }, ... ]
//	500 Internal: database failure
//
// Preconditions:
//   - Caller is authenticated and has role "admin" (enforced by middleware).
//
// Postconditions:
//   - Returns an empty array (not null) when no users exist.
func (h *UserHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.server.users.List(r.Context())
	if err != nil {
		log.Printf("users.ListUsers: repository.List: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	resp := make([]userResponse, 0, len(users))
	for _, u := range users {
		resp = append(resp, toUserResponse(u))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// DeactivateUser handles PUT /api/users/{id}/deactivate.
// Sets the user's active flag to false in PostgreSQL, then invalidates all of
// the user's active Redis sessions via SessionStore.DeleteAllForUser.
//
// URL parameter:
//
//	id — UUID of the user to deactivate.
//
// Responses:
//
//	204 No Content: user deactivated and sessions invalidated
//	400 Bad Request: id is not a valid UUID
//	404 Not Found:   no user with the given id exists
//	500 Internal:    database or Redis failure
//
// Preconditions:
//   - Caller is authenticated and has role "admin" (enforced by middleware).
//
// Postconditions:
//   - On 204: user.Active = false in the database; no active sessions remain for this user.
//   - Deactivation does NOT cancel the user's in-flight tasks (REQ-020 invariant).
func (h *UserHandler) DeactivateUser(w http.ResponseWriter, r *http.Request) {
	rawID := chi.URLParam(r, "id")
	userID, err := uuid.Parse(rawID)
	if err != nil {
		http.Error(w, "invalid user id", http.StatusBadRequest)
		return
	}

	// Verify the user exists before attempting deactivation.
	existing, err := h.server.users.GetByID(r.Context(), userID)
	if err != nil {
		log.Printf("users.DeactivateUser: repository.GetByID(%s): %v", userID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if existing == nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	// Set active=false in the database.
	if err := h.server.users.Deactivate(r.Context(), userID); err != nil {
		log.Printf("users.DeactivateUser: repository.Deactivate(%s): %v", userID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Invalidate all of the user's active sessions in Redis.
	if h.server.sessions != nil {
		if err := h.server.sessions.DeleteAllForUser(r.Context(), userID.String()); err != nil {
			log.Printf("users.DeactivateUser: sessions.DeleteAllForUser(%s): %v", userID, err)
			// Non-fatal: user is already deactivated in the DB; log and continue.
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// buildUserRoutes constructs a chi router with auth + admin role middleware applied
// to all user management endpoints. Used both in server.Handler and in tests.
//
// Preconditions:
//   - s.sessions must be non-nil for production use (nil is accepted in unit tests
//     where the middleware stack is bypassed by injecting contexts directly).
func buildUserRoutes(s *Server) http.Handler {
	r := chi.NewRouter()
	if s.sessions != nil {
		r.Use(auth.Middleware(s.sessions))
	}
	r.Use(auth.RequireRole(models.RoleAdmin))

	h := &UserHandler{server: s}
	r.Post("/api/users", h.CreateUser)
	r.Get("/api/users", h.ListUsers)
	r.Put("/api/users/{id}/deactivate", h.DeactivateUser)
	return r
}

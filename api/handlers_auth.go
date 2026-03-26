// Package api — Auth handlers: login and logout.
// POST /api/auth/login   — issues a session token as both an HTTP-only cookie and a Bearer token response body.
// POST /api/auth/logout  — invalidates the session.
// See: ADR-006, TASK-003
package api

import "net/http"

// AuthHandler handles authentication endpoints.
// Depends on UserRepository (credential lookup) and SessionStore (session lifecycle).
// See: ADR-006, TASK-003
type AuthHandler struct {
	server *Server
}

// Login handles POST /api/auth/login.
// Validates username/password against bcrypt hash (ADR-006).
// On success: generates a 256-bit session token, stores it in Redis, returns it
// in an HTTP-only Secure cookie and in the response body as { "token": "..." }.
//
// Request body:
//   { "username": "string", "password": "string" }
//
// Responses:
//   200 OK:           { "token": "...", "user": { id, username, role } }
//   400 Bad Request:  malformed JSON body
//   401 Unauthorized: username not found or password does not match
//   500 Internal:     Redis or database failure
//
// Preconditions:
//   - Request body is valid JSON with non-empty username and password.
//
// Postconditions:
//   - On 200: session exists in Redis with 24h TTL; cookie is set on the response.
//   - On 401: no session is created; no cookie is set.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement in TASK-003
	panic("not implemented")
}

// Logout handles POST /api/auth/logout.
// Reads the session token from cookie or Authorization header, deletes it from Redis.
// Always returns 204 — even if the session was already expired or not found.
//
// Responses:
//   204 No Content: session deleted (or was already absent)
//   401:            no token present in request
//
// Postconditions:
//   - On 204: the session token is no longer valid for subsequent requests.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement in TASK-003
	panic("not implemented")
}

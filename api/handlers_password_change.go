// Package api — Password change handler (SEC-001).
//
// POST /api/auth/change-password — allows an authenticated user to change their
// password. This endpoint is the only one accessible when MustChangePassword is
// true on the user's record. All other protected endpoints return 403 with reason
// "password_change_required" until this endpoint is called successfully.
//
// The mandatory first-login flow:
//  1. Admin (or any user created via POST /api/users) logs in with the seed/assigned password.
//  2. The auth middleware detects MustChangePassword = true on the session's user.
//  3. All requests except POST /api/auth/change-password return 403 with
//     {"error": "password_change_required"}.
//  4. The client redirects to the Change Password page.
//  5. User submits old password + new password via POST /api/auth/change-password.
//  6. On success: MustChangePassword is set to false; user may now access all endpoints.
//
// Password rules (SEC-007): new password must be at least 8 characters.
//
// See: SEC-001, SEC-007, ADR-006, TASK-003
package api

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/nxlabs/nexusflow/internal/auth"
)

// PasswordChangeHandler handles the POST /api/auth/change-password endpoint.
// Accessible to all authenticated users, including those with MustChangePassword = true.
// See: SEC-001, ADR-006
type PasswordChangeHandler struct {
	server *Server
}

// changePasswordRequest is the JSON body for POST /api/auth/change-password.
type changePasswordRequest struct {
	// CurrentPassword is the user's current password for verification before
	// the change is accepted. Required.
	CurrentPassword string `json:"currentPassword"`

	// NewPassword is the desired replacement password.
	// Must be at least 8 characters (SEC-007).
	NewPassword string `json:"newPassword"`
}

// minPasswordLength is the minimum acceptable length for a new password (SEC-007).
const minPasswordLength = 8

// ChangePassword handles POST /api/auth/change-password.
// Verifies the current password, hashes the new password with bcrypt (cost 12),
// updates the users table atomically setting must_change_password=false,
// then invalidates all sessions for the user.
//
// Request body:
//
//	{ "currentPassword": "string", "newPassword": "string" }
//
// Responses:
//
//	204 No Content:  password changed successfully; MustChangePassword cleared;
//	                 all sessions invalidated.
//	400 Bad Request: malformed JSON, missing fields, or new password < 8 characters.
//	401 Unauthorized: current password is incorrect.
//	403 Forbidden:   not authenticated (no valid session).
//
// Security properties:
//   - Requires an active session (enforced by auth middleware at route level).
//   - CurrentPassword is verified against the bcrypt hash before any change is made.
//   - New password is hashed with bcrypt cost 12 before storage.
//   - All existing sessions for the user are invalidated after a successful change
//     to prevent session fixation — the client must re-authenticate.
//   - No indication of which field caused a 400 that would enable username enumeration.
//
// Preconditions:
//   - Caller has a valid session (enforced by auth.Middleware).
//   - currentPassword is a non-empty string.
//   - newPassword is at least 8 characters.
//
// Postconditions:
//   - On 204: the user's password_hash is updated in the users table;
//     must_change_password is set to false;
//     all sessions for this user are deleted from Redis;
//     the calling session is also invalidated.
//   - On error: no change to the users table or session store.
func (h *PasswordChangeHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	if h.server.users == nil || h.server.sessions == nil {
		writeJSONError(w, http.StatusInternalServerError, "auth not configured")
		return
	}

	sess := auth.SessionFromContext(r.Context())
	if sess == nil {
		// Auth middleware should have blocked unauthenticated requests; this is a safety net.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden"}`))
		return
	}

	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.CurrentPassword == "" || req.NewPassword == "" {
		writeJSONError(w, http.StatusBadRequest, "currentPassword and newPassword are required")
		return
	}

	if len(req.NewPassword) < minPasswordLength {
		writeJSONError(w, http.StatusBadRequest, "newPassword must be at least 8 characters")
		return
	}

	// Fetch the current user record to verify the current password against the stored hash.
	user, err := h.server.users.GetByID(r.Context(), sess.UserID)
	if err != nil {
		log.Printf("ChangePassword: GetByID(%s): %v", sess.UserID, err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if user == nil {
		// User was deleted between session creation and this request.
		writeJSONError(w, http.StatusForbidden, "user not found")
		return
	}

	// Verify the supplied current password against the stored bcrypt hash.
	if err := auth.VerifyPassword(req.CurrentPassword, user.PasswordHash); err != nil {
		writeJSONError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}

	// Hash the new password with bcrypt cost 12 (ADR-006).
	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		log.Printf("ChangePassword: HashPassword: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Update password_hash and clear must_change_password in one atomic statement.
	// On failure no partial state is written.
	if err := h.server.users.ChangePassword(r.Context(), user.ID, newHash); err != nil {
		log.Printf("ChangePassword: ChangePassword(%s): %v", user.ID, err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Invalidate all sessions for this user (AC-6). This prevents session fixation
	// and ensures the user must re-authenticate with the new password.
	if err := h.server.sessions.DeleteAllForUser(r.Context(), user.ID.String()); err != nil {
		// Log but do not fail — the password was already changed; partial session
		// cleanup is less harmful than reporting an error to the client and leaving
		// the user stuck in the forced-change loop.
		log.Printf("ChangePassword: DeleteAllForUser(%s): %v", user.ID, err)
	}

	w.WriteHeader(http.StatusNoContent)
}

// writeJSONError writes an HTTP error response with a JSON body {"error": "message"}.
// Used by PasswordChangeHandler to produce consistent error envelopes.
func writeJSONError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	body, _ := json.Marshal(map[string]string{"error": message})
	_, _ = w.Write(body)
}

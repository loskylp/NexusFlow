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
	"net/http"
)

// PasswordChangeHandler handles the POST /api/auth/change-password endpoint.
// Accessible to all authenticated users, including those with MustChangePassword = true.
// See: SEC-001, ADR-006
type PasswordChangeHandler struct {
	server *Server
}

//lint:ignore U1000 scaffold placeholder for SEC-001
// changePasswordRequest is the JSON body for POST /api/auth/change-password.
type changePasswordRequest struct {
	// CurrentPassword is the user's current password for verification before
	// the change is accepted. Required.
	CurrentPassword string `json:"currentPassword"`

	// NewPassword is the desired replacement password.
	// Must be at least 8 characters (SEC-007).
	// Must not equal CurrentPassword.
	NewPassword string `json:"newPassword"`
}

// ChangePassword handles POST /api/auth/change-password.
// Verifies the current password, hashes the new password with bcrypt (cost 12),
// updates the users table, and sets MustChangePassword to false if it was true.
//
// Request body:
//
//	{ "currentPassword": "string", "newPassword": "string" }
//
// Responses:
//
//	204 No Content:  password changed successfully; MustChangePassword cleared
//	400 Bad Request: malformed JSON, missing fields, new password < 8 characters,
//	                 or new password equals current password
//	401 Unauthorized: current password is incorrect
//	403 Forbidden:   not authenticated (no valid session)
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
//             must_change_password is set to false;
//             all sessions for this user are deleted from Redis;
//             the calling session is also invalidated.
//   - On error: no change to the users table or session store.
func (h *PasswordChangeHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	// TODO: implement
	panic("not implemented")
}

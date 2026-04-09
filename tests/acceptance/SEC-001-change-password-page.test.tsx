/**
 * SEC-001 Acceptance Test — Change Password Page (frontend).
 *
 * Validates:
 *   1. ChangePasswordPage renders the three-field form.
 *   2. Submit button disabled until all fields are non-empty.
 *   3. Client-side validation: new password < 8 chars shows inline error.
 *   4. Client-side validation: passwords do not match shows inline error.
 *   5. Server 401 response shows "Current password is incorrect" inline.
 *   6. Server 400 response shows appropriate field-level error.
 *   7. Success (204): shows toast, clears auth, redirects to /login.
 *   8. User with mustChangePassword=false is redirected away from this route.
 *   9. Form inputs are disabled while submission is in progress.
 *
 * See: SEC-001, SEC-007, ADR-006
 */

// TODO: implement — requires ChangePasswordPage to be implemented first.
// Acceptance test structure:
//   - Render ChangePasswordPage within AuthContext (mustChangePassword=true).
//   - Mock POST /api/auth/change-password.
//   - Simulate form submissions and assert UI responses.

describe('SEC-001: Change Password Page', () => {
  it.todo('renders form with current password, new password, and confirm password fields')
  it.todo('submit button disabled until all fields are non-empty')
  it.todo('shows inline error when new password is shorter than 8 characters')
  it.todo('shows inline error when new password and confirm do not match')
  it.todo('calls POST /api/auth/change-password with current and new password on submit')
  it.todo('shows "Current password is incorrect" on 401 response')
  it.todo('shows field-level error on 400 response')
  it.todo('shows success toast and redirects to /login on 204 response')
  it.todo('disables all inputs while submission is in progress')
  it.todo('user with mustChangePassword=false is redirected away from /change-password')
})

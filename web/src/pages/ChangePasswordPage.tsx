/**
 * ChangePasswordPage — Full-screen forced password change view.
 *
 * Shown when the authenticated user's mustChangePassword flag is true.
 * The user cannot navigate away to any other view until a successful password
 * change clears the flag. All other API calls return 403 with
 * "password_change_required" until this is done.
 *
 * Layout:
 *   - Center card (400px wide): title, subtitle explaining why change is required,
 *     current password input, new password input, confirm new password input,
 *     submit button.
 *
 * States:
 *   - Default: empty form, submit button enabled.
 *   - Loading: submit button shows spinner, inputs disabled.
 *   - Error (wrong current password): inline error below current password field.
 *   - Error (new password too short): inline error below new password field.
 *   - Error (passwords do not match): inline error below confirm field.
 *   - Error (network): red error toast.
 *   - Success: toast "Password changed. Please log in again.", redirect to /login.
 *
 * After a successful password change, the server invalidates all sessions.
 * The client must redirect to /login so the user re-authenticates.
 *
 * Access: Authenticated users with mustChangePassword = true. Users who do
 * not have this flag set are redirected away from this route.
 *
 * Route: /change-password
 *
 * See: SEC-001, SEC-007, ADR-006, TASK-003
 */

import React from 'react'

// ---------------------------------------------------------------------------
// Sub-component contracts
// ---------------------------------------------------------------------------

/**
 * ChangePasswordFormProps feeds the password change form.
 */
interface ChangePasswordFormProps {
  /** True while the form submission is in progress. Disables all inputs. */
  isSubmitting: boolean
  /** Non-null error message to display below the current password field. */
  currentPasswordError: string | null
  /** Non-null error message to display below the new password field. */
  newPasswordError: string | null
  /** Non-null error message to display below the confirm password field. */
  confirmPasswordError: string | null
  /** Called when the form is submitted with the three field values. */
  onSubmit: (currentPassword: string, newPassword: string, confirmPassword: string) => void
}

/**
 * ChangePasswordForm renders the three-field password change form.
 *
 * Client-side validation (before calling onSubmit):
 *   - Current password: must be non-empty.
 *   - New password: must be at least 8 characters.
 *   - Confirm new password: must match new password.
 *
 * The Enter key in any field submits the form.
 * Tab order: current password -> new password -> confirm -> submit button.
 *
 * Postconditions:
 *   - onSubmit is not called if any client-side validation fails.
 *   - All inputs are disabled while isSubmitting is true.
 *   - Submit button shows inline spinner while isSubmitting is true.
 */
function ChangePasswordForm({
  isSubmitting,
  currentPasswordError,
  newPasswordError,
  confirmPasswordError,
  onSubmit,
}: ChangePasswordFormProps): React.ReactElement {
  // TODO: implement
  throw new Error('Not implemented')
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

/**
 * ChangePasswordPage is the root component for the forced password change flow.
 *
 * Orchestrates:
 *   - Redirect away from this page if mustChangePassword is false (via useAuth).
 *   - Form submission via POST /api/auth/change-password.
 *   - Server-side error handling: 400 (validation), 401 (wrong current password).
 *   - On success: show toast, clear auth context, redirect to /login.
 *
 * The page does not render the sidebar navigation. It is a full-screen gate
 * that the user cannot bypass until the password is changed.
 *
 * Route: /change-password
 *
 * Preconditions:
 *   - User is authenticated (non-null session in useAuth context).
 *   - User's mustChangePassword is true (enforced by ProtectedRoute logic).
 *
 * Postconditions:
 *   - On successful submission: session is cleared on the server; client is
 *     redirected to /login with a success message.
 *   - On 401 from server: "Current password is incorrect" shown inline.
 *   - On 400 from server: appropriate field-level error shown inline.
 */
function ChangePasswordPage(): React.ReactElement {
  // TODO: implement
  throw new Error('Not implemented')
}

export default ChangePasswordPage

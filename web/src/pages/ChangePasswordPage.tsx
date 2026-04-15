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
 *   - Default: empty form, submit button enabled when fields are non-empty.
 *   - Loading: submit button shows spinner text, inputs disabled.
 *   - Error (wrong current password): inline error below current password field.
 *   - Error (new password too short): inline error below new password field.
 *   - Error (passwords do not match): inline error below confirm field.
 *   - Error (network): red error below submit button.
 *   - Success: redirect to /login (server invalidates all sessions on change).
 *
 * After a successful password change, the server invalidates all sessions.
 * The client clears local auth state and redirects to /login so the user
 * re-authenticates with the new password.
 *
 * Access: Authenticated users with mustChangePassword = true. Users who do
 * not have this flag set are redirected away from this route by ProtectedRoute.
 *
 * Route: /change-password
 *
 * See: SEC-001, SEC-007, ADR-006, TASK-003
 */

import React, { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '@/context/AuthContext'
import * as client from '@/api/client'

/** Minimum acceptable length for the new password (SEC-007). */
const MIN_PASSWORD_LENGTH = 8

/**
 * ChangePasswordPage is the root component for the forced password change flow.
 *
 * Orchestrates:
 *   - Client-side validation before submission (length, match).
 *   - Form submission via POST /api/auth/change-password.
 *   - Server-side error mapping: 401 → inline "current password is incorrect",
 *     400 → inline length error.
 *   - On success: clear auth context, redirect to /login.
 *
 * The page does not render the sidebar navigation. It is a full-screen gate
 * that the user cannot bypass until the password is changed.
 *
 * Route: /change-password
 *
 * Preconditions:
 *   - User is authenticated (non-null session in useAuth context).
 *   - Accessible even when mustChangePassword is true (via allowMustChangePassword prop on ProtectedRoute).
 *
 * Postconditions:
 *   - On successful submission: session is cleared on the server; client is
 *     redirected to /login.
 *   - On 401 from server: "Current password is incorrect" shown inline below
 *     the current password field.
 *   - On 400 from server: password length error shown inline below new password field.
 */
function ChangePasswordPage(): React.ReactElement {
  const { logout } = useAuth()
  const navigate = useNavigate()

  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')

  const [currentPasswordError, setCurrentPasswordError] = useState<string | null>(null)
  const [newPasswordError, setNewPasswordError] = useState<string | null>(null)
  const [confirmPasswordError, setConfirmPasswordError] = useState<string | null>(null)
  const [networkError, setNetworkError] = useState<string | null>(null)

  const [isSubmitting, setIsSubmitting] = useState(false)

  /**
   * validateClient performs client-side validation before sending the request.
   * Returns true when all fields are valid, false otherwise.
   * Sets field-level error state for each failing check.
   */
  const validateClient = (): boolean => {
    let valid = true

    if (newPassword.length < MIN_PASSWORD_LENGTH) {
      setNewPasswordError(`Password must be at least ${MIN_PASSWORD_LENGTH} characters.`)
      valid = false
    } else {
      setNewPasswordError(null)
    }

    if (newPassword !== confirmPassword) {
      setConfirmPasswordError('Passwords do not match.')
      valid = false
    } else {
      setConfirmPasswordError(null)
    }

    return valid
  }

  /**
   * handleSubmit runs client-side validation then submits the change request.
   * Maps server error codes to inline field errors.
   */
  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    setCurrentPasswordError(null)
    setNetworkError(null)

    if (!validateClient()) {
      return
    }

    setIsSubmitting(true)
    try {
      await client.changePassword(currentPassword, newPassword)
      // Success: server has invalidated all sessions.
      // Clear auth state locally and redirect to /login for re-authentication.
      await logout()
      navigate('/login', { replace: true })
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err)
      if (message.startsWith('401')) {
        setCurrentPasswordError('Current password is incorrect.')
      } else if (message.startsWith('400')) {
        setNewPasswordError(`Password must be at least ${MIN_PASSWORD_LENGTH} characters.`)
      } else {
        setNetworkError('An error occurred. Please try again.')
      }
    } finally {
      setIsSubmitting(false)
    }
  }

  const isFormFilled = currentPassword !== '' && newPassword !== '' && confirmPassword !== ''

  return (
    <div style={styles.page}>
      <div style={styles.card}>
        {/* Header */}
        <div style={styles.header}>
          <h1 style={styles.title}>Change Password</h1>
          <p style={styles.subtitle}>
            Your account requires a password change before you can continue.
          </p>
        </div>

        {/* Form */}
        <form onSubmit={handleSubmit} noValidate>
          {/* Current Password */}
          <div style={styles.field}>
            <label htmlFor="currentPassword" style={styles.label}>
              Current Password
            </label>
            <input
              id="currentPassword"
              type="password"
              autoComplete="current-password"
              value={currentPassword}
              onChange={e => setCurrentPassword(e.target.value)}
              disabled={isSubmitting}
              required
              style={{
                ...styles.input,
                ...(isSubmitting ? styles.inputDisabled : {}),
                ...(currentPasswordError ? styles.inputError : {}),
              }}
            />
            {currentPasswordError && (
              <p role="alert" aria-live="polite" style={styles.fieldError}>
                {currentPasswordError}
              </p>
            )}
          </div>

          {/* New Password */}
          <div style={styles.field}>
            <label htmlFor="newPassword" style={styles.label}>
              New Password
            </label>
            <input
              id="newPassword"
              type="password"
              autoComplete="new-password"
              value={newPassword}
              onChange={e => setNewPassword(e.target.value)}
              disabled={isSubmitting}
              required
              style={{
                ...styles.input,
                ...(isSubmitting ? styles.inputDisabled : {}),
                ...(newPasswordError ? styles.inputError : {}),
              }}
            />
            {newPasswordError && (
              <p role="alert" aria-live="polite" style={styles.fieldError}>
                {newPasswordError}
              </p>
            )}
          </div>

          {/* Confirm New Password */}
          <div style={styles.field}>
            <label htmlFor="confirmPassword" style={styles.label}>
              Confirm New Password
            </label>
            <input
              id="confirmPassword"
              type="password"
              autoComplete="new-password"
              value={confirmPassword}
              onChange={e => setConfirmPassword(e.target.value)}
              disabled={isSubmitting}
              required
              style={{
                ...styles.input,
                ...(isSubmitting ? styles.inputDisabled : {}),
                ...(confirmPasswordError ? styles.inputError : {}),
              }}
            />
            {confirmPasswordError && (
              <p role="alert" aria-live="polite" style={styles.fieldError}>
                {confirmPasswordError}
              </p>
            )}
          </div>

          {/* Network error */}
          {networkError && (
            <p role="alert" style={styles.networkError}>
              {networkError}
            </p>
          )}

          <button
            type="submit"
            disabled={isSubmitting || !isFormFilled}
            style={{
              ...styles.button,
              ...(isSubmitting || !isFormFilled ? styles.buttonDisabled : {}),
            }}
          >
            {isSubmitting ? 'Changing...' : 'Change Password'}
          </button>
        </form>
      </div>
    </div>
  )
}

/** Inline style constants aligned with the NexusFlow design system (slate-900 background). */
const styles: Record<string, React.CSSProperties> = {
  page: {
    minHeight: '100vh',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: '#0F172A', // slate-900
  },
  card: {
    width: '400px',
    backgroundColor: '#FFFFFF',
    border: '1px solid #E2E8F0',
    borderRadius: '8px',
    padding: '40px',
  },
  header: {
    textAlign: 'center' as const,
    marginBottom: '32px',
  },
  title: {
    fontSize: '24px',
    fontWeight: 600,
    color: '#0F172A',
    margin: 0,
    fontFamily: 'var(--font-sans)',
  },
  subtitle: {
    fontSize: '13px',
    color: '#64748B',
    marginTop: '8px',
    fontFamily: 'var(--font-label)',
  },
  field: {
    marginBottom: '16px',
  },
  label: {
    display: 'block',
    fontFamily: 'var(--font-label)',
    fontSize: '12px',
    fontWeight: 500,
    textTransform: 'uppercase' as const,
    letterSpacing: '0.05em',
    color: '#64748B',
    marginBottom: '6px',
  },
  input: {
    display: 'block',
    width: '100%',
    height: '40px',
    padding: '0 12px',
    backgroundColor: '#F1F5F9',
    border: '1px solid #E2E8F0',
    borderRadius: '8px',
    fontSize: '14px',
    color: '#0F172A',
    fontFamily: 'var(--font-sans)',
    boxSizing: 'border-box' as const,
    outline: 'none',
  },
  inputDisabled: {
    opacity: 0.6,
    cursor: 'not-allowed' as const,
  },
  inputError: {
    borderColor: '#DC2626',
    backgroundColor: '#FFF5F5',
  },
  fieldError: {
    fontSize: '12px',
    color: '#DC2626',
    margin: '4px 0 0 0',
  },
  networkError: {
    fontSize: '13px',
    color: '#DC2626',
    backgroundColor: '#FEF2F2',
    border: '1px solid #FECACA',
    borderRadius: '6px',
    padding: '8px 12px',
    marginBottom: '16px',
  },
  button: {
    display: 'block',
    width: '100%',
    height: '40px',
    backgroundColor: '#4F46E5',
    color: '#FFFFFF',
    border: 'none',
    borderRadius: '8px',
    fontSize: '14px',
    fontWeight: 600,
    fontFamily: 'var(--font-sans)',
    cursor: 'pointer',
    marginTop: '8px',
  },
  buttonDisabled: {
    opacity: 0.6,
    cursor: 'not-allowed' as const,
  },
}

export default ChangePasswordPage

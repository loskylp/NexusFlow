/**
 * LoginPage — the unauthenticated entry point to NexusFlow.
 * Renders the login form per the UX spec (process/designer/ux-spec.md, Login Screen section).
 *
 * Layout: centered 400px card on a full-height slate-900 background.
 * Form fields: Username (label visible for accessibility), Password.
 * Error handling: inline error message with role="alert" below the form on failure.
 * Loading state: button shows spinner text and inputs are disabled.
 * On success: navigates to /workers (Admin) or /tasks (User) based on returned user role.
 *
 * See: TASK-019, AC-1, AC-2, AC-3
 */

import React, { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '@/context/AuthContext'

/**
 * LoginPage renders the authentication form.
 * Calls AuthContext.login() on submit.
 * Role-based redirect (admin -> /workers, user -> /tasks) is triggered by a
 * useEffect that fires when the user state is set after successful login.
 * Using useEffect avoids the React "setState in render" warning that would
 * occur if navigate() were called synchronously in the render body.
 */
function LoginPage(): React.ReactElement {
  const { login, isLoading, user } = useAuth()
  const navigate = useNavigate()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)

  // Redirect once the user state is populated after a successful login.
  // Admin -> /workers (fleet health first), User -> /tasks (own work first).
  // See: ux-spec.md — Navigation Structure, design decision 4.
  useEffect(() => {
    if (user) {
      const destination = user.role === 'admin' ? '/workers' : '/tasks'
      navigate(destination, { replace: true })
    }
  }, [user, navigate])

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    setError(null)
    try {
      await login(username, password)
      // Success: AuthContext sets user in state, triggering the useEffect above.
    } catch {
      setError('Invalid username or password.')
    }
  }

  return (
    <div style={styles.page}>
      <div style={styles.card}>
        {/* Brand */}
        <div style={styles.brand}>
          <h1 style={styles.brandName}>NexusFlow</h1>
          <p style={styles.brandSubtitle}>Task Orchestration Platform</p>
        </div>

        {/* Form */}
        <form onSubmit={handleSubmit} noValidate>
          <div style={styles.field}>
            <label htmlFor="username" style={styles.label}>
              Username
            </label>
            <input
              id="username"
              type="text"
              autoComplete="username"
              value={username}
              onChange={e => setUsername(e.target.value)}
              disabled={isLoading}
              required
              style={{
                ...styles.input,
                ...(isLoading ? styles.inputDisabled : {}),
              }}
            />
          </div>

          <div style={styles.field}>
            <label htmlFor="password" style={styles.label}>
              Password
            </label>
            <input
              id="password"
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={e => setPassword(e.target.value)}
              disabled={isLoading}
              required
              style={{
                ...styles.input,
                ...(isLoading ? styles.inputDisabled : {}),
              }}
            />
          </div>

          {/* Inline error message (AC-3) */}
          {error && (
            <p role="alert" style={styles.error}>
              {error}
            </p>
          )}

          <button
            type="submit"
            disabled={isLoading}
            style={{
              ...styles.button,
              ...(isLoading ? styles.buttonDisabled : {}),
            }}
          >
            {isLoading ? 'Signing in...' : 'Sign In'}
          </button>
        </form>

        {/* Version */}
        <p style={styles.version}>v0.1.0</p>
      </div>
    </div>
  )
}

/** Inline style constants keep the component self-contained and token-aligned. */
const styles: Record<string, React.CSSProperties> = {
  page: {
    minHeight: '100vh',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: '#0F172A', // slate-900 per UX spec
  },
  card: {
    width: '400px',
    backgroundColor: '#FFFFFF',
    border: '1px solid #E2E8F0',
    borderRadius: '8px',
    padding: '40px',
  },
  brand: {
    textAlign: 'center',
    marginBottom: '32px',
  },
  brandName: {
    fontSize: '24px',
    fontWeight: 600,
    color: '#0F172A',
    margin: 0,
    fontFamily: 'var(--font-sans)',
  },
  brandSubtitle: {
    fontSize: '13px',
    color: '#64748B',
    marginTop: '4px',
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
    cursor: 'not-allowed',
  },
  error: {
    fontSize: '13px',
    color: '#DC2626',
    backgroundColor: '#FEF2F2',
    border: '1px solid #FECACA',
    borderRadius: '6px',
    padding: '8px 12px',
    marginBottom: '16px',
    margin: '0 0 16px 0',
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
    cursor: 'not-allowed',
  },
  version: {
    textAlign: 'center' as const,
    fontSize: '11px',
    color: '#94A3B8',
    marginTop: '24px',
    fontFamily: 'var(--font-label)',
  },
}

export default LoginPage

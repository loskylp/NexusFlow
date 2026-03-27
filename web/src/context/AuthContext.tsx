/**
 * AuthContext — provides authentication state and login/logout actions to the React app.
 * Session token is stored in an HTTP-only cookie (set by the server, ADR-006).
 * The User object is kept in React state after login; the token is never accessed via JS.
 *
 * Initial auth check: the context performs a GET /api/auth/me on mount to restore
 * a pre-existing session. Until the check completes, isLoading is true so that
 * ProtectedRoute does not flash a redirect to /login.
 *
 * See: ADR-006, TASK-019
 */

import React, { createContext, useContext, useEffect, useState } from 'react'
import type { User } from '@/types/domain'
import * as client from '@/api/client'

interface AuthContextValue {
  /** The authenticated user, or null if not logged in. */
  user: User | null

  /**
   * Attempt to log in with the given credentials.
   * On success, stores the user in context.
   * On failure, throws an error with the HTTP status prefix so the caller can
   * display an appropriate inline error message.
   *
   * @param username - Plain-text username.
   * @param password - Plain-text password.
   * @throws Error with message starting '401' on invalid credentials.
   */
  login: (username: string, password: string) => Promise<void>

  /**
   * Log out the current user.
   * Calls POST /api/auth/logout to invalidate the server-side session.
   * Clears user from context regardless of server response so the UI
   * always reflects the logged-out state.
   */
  logout: () => Promise<void>

  /**
   * True while an auth check, login, or logout request is in flight.
   * ProtectedRoute uses this to avoid a redirect flash on page load.
   */
  isLoading: boolean
}

const AuthContext = createContext<AuthContextValue | null>(null)

/**
 * AuthProvider wraps the application and provides auth state to all descendants.
 * Place at the top of the component tree, above the Router.
 *
 * On mount, attempts to restore an existing session via GET /api/auth/me.
 * If the cookie is valid the server returns the User object; otherwise the
 * user starts as null (unauthenticated).
 *
 * See: TASK-019
 */
export function AuthProvider({ children }: { children: React.ReactNode }): React.ReactElement {
  const [user, setUser] = useState<User | null>(null)
  const [isLoading, setIsLoading] = useState(true)

  // Restore session on mount by checking a /api/auth/me endpoint.
  // If the endpoint is not yet implemented (Cycle 1), the request will 404
  // and we silently remain unauthenticated.
  useEffect(() => {
    fetch('/api/auth/me', { credentials: 'include' })
      .then(res => {
        if (res.ok) return res.json() as Promise<{ user: User }>
        return null
      })
      .then(data => {
        if (data) setUser(data.user)
      })
      .catch(() => {
        // Network error or 404 (endpoint not yet live) — remain unauthenticated.
      })
      .finally(() => setIsLoading(false))
  }, [])

  /**
   * login calls the API client, extracts the user from the response,
   * and stores it in context. Propagates any error from the client so
   * LoginPage can display the inline error.
   */
  const login = async (username: string, password: string): Promise<void> => {
    setIsLoading(true)
    try {
      const response = await client.login(username, password)
      setUser(response.user)
    } finally {
      setIsLoading(false)
    }
  }

  /**
   * logout invalidates the server session and clears the user from context.
   * User is cleared even if the server call fails (cookie may already be gone).
   */
  const logout = async (): Promise<void> => {
    setIsLoading(true)
    try {
      await client.logout()
    } catch {
      // Ignore logout errors — session may already be expired.
    } finally {
      setUser(null)
      setIsLoading(false)
    }
  }

  return (
    <AuthContext.Provider value={{ user, login, logout, isLoading }}>
      {children}
    </AuthContext.Provider>
  )
}

/**
 * useAuth returns the AuthContext value.
 * Must be called within a component tree wrapped by AuthProvider.
 *
 * @throws Error if called outside an AuthProvider.
 */
export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within AuthProvider')
  return ctx
}

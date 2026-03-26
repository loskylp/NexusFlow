/**
 * AuthContext — provides authentication state and login/logout actions to the React app.
 * Session token is stored in an HTTP-only cookie (set by the server, ADR-006).
 * The User object is kept in React state after login; the token is never accessed via JS.
 *
 * See: ADR-006, TASK-019
 */

import React, { createContext, useContext, useState } from 'react'
import type { User } from '@/types/domain'

interface AuthContextValue {
  /** The authenticated user, or null if not logged in. */
  user: User | null

  /**
   * Attempt to log in with the given credentials.
   * On success, stores the user in context and navigates to the landing page.
   * On failure, throws an error with a user-readable message.
   *
   * @param username - Plain-text username.
   * @param password - Plain-text password.
   */
  login: (username: string, password: string) => Promise<void>

  /**
   * Log out the current user.
   * Calls POST /api/auth/logout to invalidate the server-side session.
   * Clears user from context and navigates to /login.
   */
  logout: () => Promise<void>

  /** True while a login or logout request is in flight. */
  isLoading: boolean
}

const AuthContext = createContext<AuthContextValue | null>(null)

/**
 * AuthProvider wraps the application and provides auth state to all descendants.
 * Place at the top of the component tree, above the Router.
 * See: TASK-019
 */
export function AuthProvider({ children }: { children: React.ReactNode }): React.ReactElement {
  // TODO: Implement in TASK-019
  const [user, _setUser] = useState<User | null>(null)
  const [isLoading, _setIsLoading] = useState(false)

  const login = async (_username: string, _password: string): Promise<void> => {
    // TODO: Implement in TASK-019
    throw new Error('Not implemented')
  }

  const logout = async (): Promise<void> => {
    // TODO: Implement in TASK-019
    throw new Error('Not implemented')
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
 * Throws if called outside an AuthProvider.
 */
export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within AuthProvider')
  return ctx
}

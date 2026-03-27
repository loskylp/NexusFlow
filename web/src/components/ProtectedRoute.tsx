/**
 * ProtectedRoute — wraps authenticated routes.
 * Redirects unauthenticated users to /login (AC-6).
 * Renders nothing while the initial auth check is in flight (isLoading) to
 * prevent a flash of /login on hard refresh when a valid session cookie exists.
 *
 * See: TASK-019
 */

import React from 'react'
import { Navigate } from 'react-router-dom'
import { useAuth } from '@/context/AuthContext'

interface ProtectedRouteProps {
  /** The protected page content to render when authenticated. */
  children: React.ReactNode
}

/**
 * ProtectedRoute renders its children when the user is authenticated.
 * While the auth state is loading, renders null to avoid a spurious redirect.
 * When loading completes and user is null, redirects to /login.
 *
 * Preconditions:
 *   - Must be rendered inside an AuthProvider and a Router.
 * Postconditions:
 *   - If user is authenticated: children are rendered.
 *   - If auth check is in flight: nothing is rendered.
 *   - If user is unauthenticated: Navigate to /login is rendered.
 */
function ProtectedRoute({ children }: ProtectedRouteProps): React.ReactElement | null {
  const { user, isLoading } = useAuth()

  if (isLoading) {
    // Suppress the redirect flash while the session check is in flight.
    return null
  }

  if (!user) {
    return <Navigate to="/login" replace />
  }

  return <>{children}</>
}

export default ProtectedRoute

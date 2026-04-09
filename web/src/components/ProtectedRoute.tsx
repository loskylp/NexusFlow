/**
 * ProtectedRoute — wraps authenticated routes.
 * Redirects unauthenticated users to /login (AC-6).
 * Renders nothing while the initial auth check is in flight (isLoading) to
 * prevent a flash of /login on hard refresh when a valid session cookie exists.
 *
 * SEC-001 extension: when the authenticated user has mustChangePassword = true,
 * all routes except /change-password redirect to /change-password. The
 * allowMustChangePassword prop opts the route out of this redirect (used only
 * for the /change-password route itself).
 *
 * See: TASK-019, SEC-001
 */

import React from 'react'
import { Navigate } from 'react-router-dom'
import { useAuth } from '@/context/AuthContext'

interface ProtectedRouteProps {
  /** The protected page content to render when authenticated. */
  children: React.ReactNode
  /**
   * When true, the route is accessible even when the user has mustChangePassword=true.
   * Should only be set on the /change-password route itself. Defaults to false.
   * See: SEC-001
   */
  allowMustChangePassword?: boolean
}

/**
 * ProtectedRoute renders its children when the user is authenticated.
 * While the auth state is loading, renders null to avoid a spurious redirect.
 * When loading completes and user is null, redirects to /login.
 *
 * When the authenticated user has mustChangePassword = true AND
 * allowMustChangePassword is false (default), redirects to /change-password.
 * This enforces the mandatory first-login password change from SEC-001.
 *
 * Preconditions:
 *   - Must be rendered inside an AuthProvider and a Router.
 *
 * Postconditions:
 *   - If user is unauthenticated: Navigate to /login.
 *   - If user is authenticated and mustChangePassword=true and allowMustChangePassword=false:
 *     Navigate to /change-password.
 *   - If user is authenticated (and password change not required): children are rendered.
 *   - If auth check is in flight: nothing is rendered (suppresses redirect flash).
 */
function ProtectedRoute({
  children,
  allowMustChangePassword = false,
}: ProtectedRouteProps): React.ReactElement | null {
  const { user, isLoading } = useAuth()

  if (isLoading) {
    // Suppress the redirect flash while the session check is in flight.
    return null
  }

  if (!user) {
    return <Navigate to="/login" replace />
  }

  // SEC-001: force password change before any other route is accessible.
  if (user.mustChangePassword && !allowMustChangePassword) {
    return <Navigate to="/change-password" replace />
  }

  return <>{children}</>
}

export default ProtectedRoute

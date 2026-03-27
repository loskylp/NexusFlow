/**
 * Layout — shell for all authenticated views.
 * Renders the Sidebar on the left and the page content on the right.
 * The main content area is offset by the sidebar width (var(--sidebar-width)).
 *
 * See: TASK-019, DESIGN.md — Sidebar Navigation
 */

import React from 'react'
import Sidebar from './Sidebar'

interface LayoutProps {
  /** The page content to render in the main area. */
  children: React.ReactNode
}

/**
 * Layout wraps authenticated page content with the global sidebar.
 * It should only be used inside ProtectedRoute to guarantee the user is logged in
 * before Sidebar attempts to read from AuthContext.
 */
function Layout({ children }: LayoutProps): React.ReactElement {
  return (
    <div style={{ display: 'flex', minHeight: '100vh' }}>
      <Sidebar />
      <main
        style={{
          marginLeft: 'var(--sidebar-width)',
          flex: 1,
          padding: 'var(--space-6)',
          backgroundColor: 'var(--color-surface-base)',
          minHeight: '100vh',
        }}
      >
        {children}
      </main>
    </div>
  )
}

export default Layout

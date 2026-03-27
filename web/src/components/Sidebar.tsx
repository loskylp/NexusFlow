/**
 * Sidebar — fixed left navigation for all authenticated views.
 * Width: 240px (var(--sidebar-width)), dark slate-900 background.
 * Renders primary nav items for all authenticated users.
 * Renders Demo section (Sink Inspector, Chaos Controller) for Admin only (AC-5).
 * Active item is highlighted with an indigo left border per DESIGN.md.
 * Includes logout button at the bottom.
 *
 * See: TASK-019, ux-spec.md — Navigation Structure, DESIGN.md — Sidebar Navigation
 */

import React from 'react'
import { NavLink, useNavigate } from 'react-router-dom'
import { useAuth } from '@/context/AuthContext'

interface NavItem {
  label: string
  path: string
}

/** Primary navigation items visible to all authenticated roles. */
const PRIMARY_NAV: NavItem[] = [
  { label: 'Worker Fleet', path: '/workers' },
  { label: 'Task Feed', path: '/tasks' },
  { label: 'Pipeline Builder', path: '/pipelines' },
  { label: 'Log Streamer', path: '/tasks/logs' },
]

/** Demo section items visible to Admin role only (AC-5). */
const DEMO_NAV: NavItem[] = [
  { label: 'Sink Inspector', path: '/demo/sink-inspector' },
  { label: 'Chaos Controller', path: '/demo/chaos' },
]

/**
 * Sidebar renders the fixed left navigation rail.
 * Role-based demo section visibility is determined by the authenticated user's role.
 * The logout button calls AuthContext.logout() and navigates to /login.
 *
 * Preconditions:
 *   - Must be rendered inside an AuthProvider and a Router.
 *   - useAuth().user is non-null (Sidebar should only mount inside ProtectedRoute).
 */
function Sidebar(): React.ReactElement {
  const { user, logout } = useAuth()
  const navigate = useNavigate()

  const handleLogout = async () => {
    await logout()
    navigate('/login', { replace: true })
  }

  return (
    <nav
      className="sidebar"
      role="navigation"
      aria-label="Main navigation"
      style={{
        width: 'var(--sidebar-width)',
        minHeight: '100vh',
        backgroundColor: 'var(--sidebar-bg)',
        display: 'flex',
        flexDirection: 'column',
        position: 'fixed',
        top: 0,
        left: 0,
        bottom: 0,
        zIndex: 10,
      }}
    >
      {/* Brand */}
      <div className="sidebar__brand" style={{ padding: '24px 20px 20px', borderBottom: '1px solid rgba(255,255,255,0.08)' }}>
        <span style={{ color: '#FFFFFF', fontWeight: 600, fontSize: '16px', letterSpacing: '-0.01em' }}>
          NexusFlow
        </span>
      </div>

      {/* Primary navigation */}
      <ul
        style={{ listStyle: 'none', padding: '8px 0', margin: 0, flex: 1 }}
        role="list"
      >
        {PRIMARY_NAV.map(item => (
          <li key={item.path}>
            <NavLink
              to={item.path}
              style={({ isActive }) => navLinkStyle(isActive)}
            >
              {item.label}
            </NavLink>
          </li>
        ))}
      </ul>

      {/* Demo section — Admin only (AC-5) */}
      {user?.role === 'admin' && (
        <div style={{ borderTop: '1px solid rgba(255,255,255,0.08)', paddingTop: '8px' }}>
          <span
            style={{
              display: 'block',
              padding: '8px 20px 4px',
              color: 'var(--color-text-tertiary)',
              fontFamily: 'var(--font-label)',
              fontSize: '11px',
              textTransform: 'uppercase',
              letterSpacing: '0.08em',
            }}
          >
            Demo
          </span>
          <ul style={{ listStyle: 'none', padding: '0 0 8px', margin: 0 }} role="list">
            {DEMO_NAV.map(item => (
              <li key={item.path}>
                <NavLink
                  to={item.path}
                  style={({ isActive }) => navLinkStyle(isActive)}
                >
                  {item.label}
                </NavLink>
              </li>
            ))}
          </ul>
        </div>
      )}

      {/* Logout */}
      <div style={{ borderTop: '1px solid rgba(255,255,255,0.08)', padding: '12px 12px' }}>
        <button
          onClick={handleLogout}
          aria-label="Log out"
          style={{
            width: '100%',
            padding: '8px 8px',
            background: 'transparent',
            border: '1px solid rgba(255,255,255,0.15)',
            borderRadius: '6px',
            color: 'var(--color-text-secondary)',
            fontSize: '14px',
            cursor: 'pointer',
            textAlign: 'left',
          }}
        >
          Log out
        </button>
      </div>
    </nav>
  )
}

/**
 * navLinkStyle returns inline styles for NavLink based on active state.
 * Active state: indigo left border + indigo-50 background per DESIGN.md.
 */
function navLinkStyle(isActive: boolean): React.CSSProperties {
  return {
    display: 'block',
    padding: '9px 20px',
    color: isActive ? '#EEF2FF' : '#CBD5E1',
    backgroundColor: isActive ? 'rgba(79,70,229,0.15)' : 'transparent',
    borderLeft: isActive ? '3px solid #4F46E5' : '3px solid transparent',
    fontWeight: isActive ? 600 : 400,
    fontSize: '14px',
    textDecoration: 'none',
    transition: 'background-color 0.15s ease, color 0.15s ease',
  }
}

export default Sidebar

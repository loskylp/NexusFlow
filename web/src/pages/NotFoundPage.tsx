/**
 * NotFoundPage — 404 fallback for unmatched routes.
 * Styled per the design system (DESIGN.md).
 * See: TASK-019
 */

import React from 'react'
import { useNavigate } from 'react-router-dom'

/**
 * NotFoundPage renders a 404 message with a link back to the dashboard.
 */
function NotFoundPage(): React.ReactElement {
  const navigate = useNavigate()
  return (
    <div style={{
      minHeight: '100vh',
      display: 'flex',
      flexDirection: 'column',
      alignItems: 'center',
      justifyContent: 'center',
      backgroundColor: 'var(--color-surface-base)',
    }}>
      <h1 style={{ fontSize: '20px', fontWeight: 600, color: 'var(--color-text-primary)', marginBottom: '8px' }}>
        404 — Page Not Found
      </h1>
      <p style={{ color: 'var(--color-text-secondary)', marginBottom: '24px' }}>
        The page you requested does not exist.
      </p>
      <button
        onClick={() => navigate('/')}
        style={{
          padding: '8px 20px',
          backgroundColor: 'var(--color-primary)',
          color: '#FFFFFF',
          border: 'none',
          borderRadius: '8px',
          fontSize: '14px',
          fontWeight: 600,
          cursor: 'pointer',
        }}
      >
        Back to Dashboard
      </button>
    </div>
  )
}

export default NotFoundPage

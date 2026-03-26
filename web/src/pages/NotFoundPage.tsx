/**
 * NotFoundPage — 404 fallback for unmatched routes.
 * See: TASK-019
 */

import React from 'react'
import { useNavigate } from 'react-router-dom'

function NotFoundPage(): React.ReactElement {
  const navigate = useNavigate()
  return (
    <div>
      {/* TODO: Style in TASK-019 */}
      <h1>404 — Page Not Found</h1>
      <button onClick={() => navigate('/')}>Back to Dashboard</button>
    </div>
  )
}

export default NotFoundPage

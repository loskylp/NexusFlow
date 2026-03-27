/**
 * TaskFeedPage — placeholder for the Task Feed and Monitor view.
 * Full implementation deferred to Cycle 2 (TASK-021).
 *
 * See: TASK-019, ux-spec.md — Task Feed and Monitor
 */

import React from 'react'

/**
 * TaskFeedPage renders a placeholder until TASK-021 implements the full view.
 */
function TaskFeedPage(): React.ReactElement {
  return (
    <div>
      <h1 style={{ fontSize: '20px', fontWeight: 600, color: 'var(--color-text-primary)' }}>
        Task Feed
      </h1>
      <p style={{ color: 'var(--color-text-secondary)', marginTop: '8px' }}>
        Coming in Cycle 2 (TASK-021).
      </p>
    </div>
  )
}

export default TaskFeedPage

/**
 * ChaosControllerPage — placeholder for the Chaos Controller demo view.
 * Admin only. Full implementation deferred to Cycle 4.
 *
 * See: TASK-019, ux-spec.md — Chaos Controller
 */

import React from 'react'

/**
 * ChaosControllerPage renders a placeholder until the Chaos Controller is implemented.
 */
function ChaosControllerPage(): React.ReactElement {
  return (
    <div>
      <h1 style={{ fontSize: '20px', fontWeight: 600, color: 'var(--color-text-primary)' }}>
        Chaos Controller
        <span style={{ marginLeft: '8px', fontSize: '11px', backgroundColor: '#FEF9C3', color: '#92400E', padding: '2px 6px', borderRadius: '4px', fontWeight: 500 }}>
          DEMO
        </span>
      </h1>
      <p style={{ color: 'var(--color-text-secondary)', marginTop: '8px' }}>
        Coming in Cycle 4.
      </p>
    </div>
  )
}

export default ChaosControllerPage

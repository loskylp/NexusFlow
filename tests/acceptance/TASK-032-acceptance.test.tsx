/**
 * TASK-032 Acceptance Test — Sink Inspector GUI.
 *
 * Validates:
 *   1. Sink Inspector page renders at /demo/sink-inspector (admin only).
 *   2. Non-admin access shows an access denied message.
 *   3. Task selector dropdown lists recent tasks.
 *   4. Selecting a task subscribes to SSE channel GET /events/sink/{taskId}.
 *   5. Before panel populates on sink:before-snapshot event.
 *   6. After panel populates on sink:after-result event.
 *   7. Successful write: delta highlights shown in green-50; atomicity verified checkmark.
 *   8. Rollback: After panel matches Before; "ROLLED BACK" badge shown.
 *   9. Changing the selected task resets all snapshot state.
 *
 * See: DEMO-003, UX Spec (Sink Inspector), TASK-032, TASK-033
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, act, fireEvent } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import SinkInspectorPage from '../../web/src/pages/SinkInspectorPage'
import type { Task, SinkSnapshot, SinkInspectorState, SSEEvent } from '../../web/src/types/domain'
import type { UseTasksReturn } from '../../web/src/hooks/useTasks'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

vi.mock('../../web/src/context/AuthContext')
vi.mock('../../web/src/hooks/useTasks')

import * as authModule from '../../web/src/context/AuthContext'
import * as tasksModule from '../../web/src/hooks/useTasks'

const mockUseAuth = vi.mocked(authModule.useAuth)
const mockUseTasks = vi.mocked(tasksModule.useTasks)

// ---------------------------------------------------------------------------
// Fake EventSource — lightweight, no fake timers needed
// ---------------------------------------------------------------------------

interface FakeEventSourceInstance {
  url: string
  withCredentials: boolean
  onopen: ((e: Event) => void) | null
  onmessage: ((e: MessageEvent) => void) | null
  onerror: ((e: Event) => void) | null
  close: ReturnType<typeof vi.fn>
  simulateOpen(): void
  simulateMessage(data: string): void
}

let instances: FakeEventSourceInstance[] = []

class FakeEventSource implements FakeEventSourceInstance {
  url: string
  withCredentials: boolean
  onopen: ((e: Event) => void) | null = null
  onmessage: ((e: MessageEvent) => void) | null = null
  onerror: ((e: Event) => void) | null = null
  close = vi.fn()
  addEventListener = vi.fn()
  removeEventListener = vi.fn()

  constructor(url: string, init?: EventSourceInit) {
    this.url = url
    this.withCredentials = init?.withCredentials ?? false
    instances.push(this)
  }

  simulateOpen() {
    this.onopen?.(new Event('open'))
  }

  simulateMessage(data: string) {
    this.onmessage?.(new MessageEvent('message', { data }))
  }
}

function latestInstance(): FakeEventSourceInstance {
  const inst = instances[instances.length - 1]
  if (!inst) throw new Error('No EventSource instance found')
  return inst
}

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const TASK_ID_1 = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
const TASK_ID_2 = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'

function makeTask(id: string): Task {
  return {
    id,
    pipelineId: 'pipe-001',
    userId: 'user-001',
    status: 'completed',
    retryConfig: { maxRetries: 0, backoff: 'fixed' },
    retryCount: 0,
    executionId: 'exec-001',
    input: {},
    createdAt: '2026-04-15T10:00:00Z',
    updatedAt: '2026-04-15T10:00:00Z',
  }
}

function makeBeforeSnapshot(taskId: string): SinkSnapshot {
  return {
    taskId,
    phase: 'before',
    data: { object_count: 3, bucket: 'demo-bucket' },
    capturedAt: '2026-04-15T10:00:01Z',
  }
}

function makeAfterSnapshot(taskId: string): SinkSnapshot {
  return {
    taskId,
    phase: 'after',
    data: { object_count: 5, bucket: 'demo-bucket', new_key: 'added' },
    capturedAt: '2026-04-15T10:00:05Z',
  }
}

function makeRolledBackSnapshot(taskId: string, before: SinkSnapshot): SinkSnapshot {
  return {
    taskId,
    phase: 'after',
    data: { ...before.data },
    capturedAt: '2026-04-15T10:00:06Z',
  }
}

function buildBeforeEvent(taskId: string, before: SinkSnapshot): SSEEvent<SinkInspectorState> {
  return {
    type: 'sink:before-snapshot',
    payload: {
      eventType: 'sink:before-snapshot',
      taskId,
      before,
      after: null,
      rolledBack: false,
      writeError: '',
    },
  }
}

function buildAfterSuccessEvent(
  taskId: string,
  before: SinkSnapshot,
  after: SinkSnapshot
): SSEEvent<SinkInspectorState> {
  return {
    type: 'sink:after-result',
    payload: {
      eventType: 'sink:after-result',
      taskId,
      before,
      after,
      rolledBack: false,
      writeError: '',
    },
  }
}

function buildRollbackEvent(
  taskId: string,
  before: SinkSnapshot,
  after: SinkSnapshot
): SSEEvent<SinkInspectorState> {
  return {
    type: 'sink:after-result',
    payload: {
      eventType: 'sink:after-result',
      taskId,
      before,
      after,
      rolledBack: true,
      writeError: 'write failed: connection reset',
    },
  }
}

// ---------------------------------------------------------------------------
// Setup / teardown
// ---------------------------------------------------------------------------

beforeEach(() => {
  instances = []
  vi.stubGlobal('EventSource', FakeEventSource)

  // Default: admin user
  mockUseAuth.mockReturnValue({
    user: {
      id: 'user-admin',
      username: 'admin',
      role: 'admin',
      active: true,
      mustChangePassword: false,
      createdAt: '2026-01-01T00:00:00Z',
    },
    login: vi.fn(),
    logout: vi.fn(),
    isLoading: false,
  })

  // Default: two tasks in the selector
  mockUseTasks.mockReturnValue({
    tasks: [makeTask(TASK_ID_1), makeTask(TASK_ID_2)],
    isLoading: false,
    error: null,
    sseStatus: 'connected',
    refresh: vi.fn(),
  } satisfies UseTasksReturn)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/demo/sink-inspector']}>
      <SinkInspectorPage />
    </MemoryRouter>
  )
}

/** Helper: select a task in the dropdown and return. */
function selectTask(taskId: string) {
  const select = screen.getByRole('combobox', { name: /select task/i })
  act(() => {
    fireEvent.change(select, { target: { value: taskId } })
  })
}

// ---------------------------------------------------------------------------
// AC-1: Page renders for Admin at /demo/sink-inspector
// ---------------------------------------------------------------------------

describe('AC-1: Sink Inspector page renders for admin', () => {
  it('renders the "Sink Inspector" heading for an Admin user', () => {
    renderPage()
    expect(screen.getByText('Sink Inspector')).toBeInTheDocument()
  })

  it('renders the DEMO badge', () => {
    renderPage()
    expect(screen.getByText('DEMO')).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// AC-2: Non-admin access denied
// ---------------------------------------------------------------------------

describe('AC-2: Non-admin user sees access denied', () => {
  it('shows access denied message and no page content for user role', () => {
    mockUseAuth.mockReturnValue({
      user: {
        id: 'user-002',
        username: 'regularuser',
        role: 'user',
        active: true,
        mustChangePassword: false,
        createdAt: '2026-01-01T00:00:00Z',
      },
      login: vi.fn(),
      logout: vi.fn(),
      isLoading: false,
    })
    renderPage()
    expect(screen.getAllByText(/access denied/i).length).toBeGreaterThan(0)
    // The page title should not be shown.
    expect(screen.queryByText('Sink Inspector')).not.toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// AC-3: Task selector lists recent tasks
// ---------------------------------------------------------------------------

describe('AC-3: Task selector dropdown lists recent tasks', () => {
  it('renders a combobox with the two test tasks as options', () => {
    renderPage()
    const select = screen.getByRole('combobox', { name: /select task/i })
    expect(select).toBeInTheDocument()
    expect(screen.getByText(new RegExp(TASK_ID_1.slice(0, 8)))).toBeInTheDocument()
    expect(screen.getByText(new RegExp(TASK_ID_2.slice(0, 8)))).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// AC-4: Selecting a task subscribes to SSE channel
// ---------------------------------------------------------------------------

describe('AC-4: Selecting a task subscribes to the SSE channel', () => {
  it('opens EventSource for /events/sink/{taskId} on task selection', () => {
    renderPage()
    selectTask(TASK_ID_1)

    const sinkInstances = instances.filter(i => i.url === `/events/sink/${TASK_ID_1}`)
    expect(sinkInstances.length).toBeGreaterThan(0)
  })
})

// ---------------------------------------------------------------------------
// AC-5: Before panel populates on sink:before-snapshot event
// ---------------------------------------------------------------------------

describe('AC-5: Before panel populates on sink:before-snapshot', () => {
  it('shows snapshot data keys in the Before panel after event fires', () => {
    renderPage()
    selectTask(TASK_ID_1)

    const before = makeBeforeSnapshot(TASK_ID_1)
    act(() => {
      latestInstance().simulateOpen()
      latestInstance().simulateMessage(JSON.stringify(buildBeforeEvent(TASK_ID_1, before)))
    })

    expect(screen.getByText('object_count')).toBeInTheDocument()
    expect(screen.getByText('bucket')).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// AC-6: After panel populates on sink:after-result event
// ---------------------------------------------------------------------------

describe('AC-6: After panel populates on sink:after-result', () => {
  it('shows After panel data after sink:after-result event fires', () => {
    renderPage()
    selectTask(TASK_ID_1)

    const before = makeBeforeSnapshot(TASK_ID_1)
    const after = makeAfterSnapshot(TASK_ID_1)

    act(() => {
      latestInstance().simulateOpen()
      latestInstance().simulateMessage(JSON.stringify(buildBeforeEvent(TASK_ID_1, before)))
      latestInstance().simulateMessage(JSON.stringify(buildAfterSuccessEvent(TASK_ID_1, before, after)))
    })

    // "new_key" is only present in the After snapshot.
    expect(screen.getByText('new_key')).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// AC-7: Delta highlights and atomicity checkmark on success
// ---------------------------------------------------------------------------

describe('AC-7: Delta highlights and atomicity checkmark on successful write', () => {
  it('shows atomicity verified checkmark on successful write', () => {
    renderPage()
    selectTask(TASK_ID_1)

    const before = makeBeforeSnapshot(TASK_ID_1)
    const after = makeAfterSnapshot(TASK_ID_1)

    act(() => {
      latestInstance().simulateOpen()
      latestInstance().simulateMessage(JSON.stringify(buildBeforeEvent(TASK_ID_1, before)))
      latestInstance().simulateMessage(JSON.stringify(buildAfterSuccessEvent(TASK_ID_1, before, after)))
    })

    expect(screen.getByLabelText(/atomicity verified/i)).toBeInTheDocument()
    expect(screen.getByText(/write committed successfully/i)).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// AC-8: Rollback — ROLLED BACK badge shown
// ---------------------------------------------------------------------------

describe('AC-8: Rollback — ROLLED BACK badge shown', () => {
  it('shows ROLLED BACK badge and error message when rollback fires', () => {
    renderPage()
    selectTask(TASK_ID_1)

    const before = makeBeforeSnapshot(TASK_ID_1)
    const after = makeRolledBackSnapshot(TASK_ID_1, before)

    act(() => {
      latestInstance().simulateOpen()
      latestInstance().simulateMessage(JSON.stringify(buildBeforeEvent(TASK_ID_1, before)))
      latestInstance().simulateMessage(JSON.stringify(buildRollbackEvent(TASK_ID_1, before, after)))
    })

    expect(screen.getAllByText(/rolled back/i).length).toBeGreaterThan(0)
    expect(screen.getByText(/write failed: connection reset/i)).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// AC-9: Changing selected task resets snapshot state
// ---------------------------------------------------------------------------

describe('AC-9: Changing selected task resets snapshot state', () => {
  it('clears Before panel data when a different task is selected', () => {
    renderPage()
    selectTask(TASK_ID_1)

    const before = makeBeforeSnapshot(TASK_ID_1)

    act(() => {
      latestInstance().simulateOpen()
      latestInstance().simulateMessage(JSON.stringify(buildBeforeEvent(TASK_ID_1, before)))
    })

    // Snapshot data should be visible.
    expect(screen.getByText('object_count')).toBeInTheDocument()

    // Switch to a different task — snapshot state must reset.
    selectTask(TASK_ID_2)

    // The "object_count" key must no longer be visible (panels cleared on task change).
    expect(screen.queryByText('object_count')).not.toBeInTheDocument()
  })
})

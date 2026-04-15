/**
 * Unit tests for LoginPage.
 * Covers AC-1 (form renders), AC-2 (success redirect), AC-3 (invalid creds inline error).
 *
 * All tests use the real AuthProvider with client.login mocked at the API layer.
 * This avoids the complexity of mocking the AuthContext hook itself (which requires
 * intercepting the module-bound reference inside LoginPage).
 *
 * See: TASK-019, ux-spec.md Login screen section
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import LoginPage from './LoginPage'
import { AuthProvider } from '@/context/AuthContext'
import * as client from '@/api/client'

// Mock only the API client — AuthProvider is real
vi.mock('@/api/client')
const mockClientLogin = vi.mocked(client.login)

beforeEach(() => {
  vi.clearAllMocks()
  // Stub global fetch for AuthProvider's /api/auth/me session-restore call.
  // Return 404 so the session check resolves immediately with no prior session.
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
    ok: false,
    status: 404,
    json: async () => ({}),
  }))
})

afterEach(() => {
  vi.unstubAllGlobals()
})

/**
 * Renders LoginPage inside AuthProvider and a MemoryRouter.
 * The router includes /workers and /tasks stubs so redirect assertions work.
 */
function renderLoginPage() {
  return render(
    <AuthProvider>
      <MemoryRouter initialEntries={['/login']}>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/workers" element={<div>workers page</div>} />
          <Route path="/tasks" element={<div>tasks page</div>} />
        </Routes>
      </MemoryRouter>
    </AuthProvider>
  )
}

/**
 * Waits for the AuthProvider's initial session check to complete so that
 * isLoading becomes false and the LoginPage renders with the form fully active.
 */
async function waitForReady() {
  await waitFor(() => expect(screen.getByLabelText(/username/i)).toBeInTheDocument())
}

describe('LoginPage — form renders (AC-1)', () => {
  it('renders username input, password input, and sign-in button', async () => {
    renderLoginPage()
    await waitForReady()

    expect(screen.getByLabelText(/username/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/password/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /sign in/i })).toBeInTheDocument()
  })

  it('renders the NexusFlow brand name', async () => {
    renderLoginPage()
    await waitForReady()

    expect(screen.getByText(/nexusflow/i)).toBeInTheDocument()
  })
})

describe('LoginPage — success redirect (AC-2)', () => {
  it('redirects admin to /workers on successful login', async () => {
    mockClientLogin.mockResolvedValueOnce({
      token: 'tok-1',
      user: { id: 'u1', username: 'alice', role: 'admin', active: true, mustChangePassword: false, createdAt: '' },
    })

    renderLoginPage()
    await waitForReady()

    await userEvent.type(screen.getByLabelText(/username/i), 'alice')
    await userEvent.type(screen.getByLabelText(/password/i), 'secret')
    await userEvent.click(screen.getByRole('button', { name: /sign in/i }))

    await waitFor(() =>
      expect(screen.queryByText('workers page')).toBeInTheDocument()
    )
  })

  it('redirects user to /tasks on successful login', async () => {
    mockClientLogin.mockResolvedValueOnce({
      token: 'tok-2',
      user: { id: 'u2', username: 'bob', role: 'user', active: true, mustChangePassword: false, createdAt: '' },
    })

    renderLoginPage()
    await waitForReady()

    await userEvent.type(screen.getByLabelText(/username/i), 'bob')
    await userEvent.type(screen.getByLabelText(/password/i), 'pass')
    await userEvent.click(screen.getByRole('button', { name: /sign in/i }))

    await waitFor(() =>
      expect(screen.queryByText('tasks page')).toBeInTheDocument()
    )
  })
})

describe('LoginPage — invalid credentials (AC-3)', () => {
  it('shows inline error on login failure', async () => {
    mockClientLogin.mockRejectedValueOnce(new Error('401: invalid credentials'))

    renderLoginPage()
    await waitForReady()

    await userEvent.type(screen.getByLabelText(/username/i), 'bad')
    await userEvent.type(screen.getByLabelText(/password/i), 'wrong')
    await userEvent.click(screen.getByRole('button', { name: /sign in/i }))

    await waitFor(() =>
      expect(screen.getByRole('alert')).toHaveTextContent(/invalid username or password/i)
    )
  })

  it('does not navigate away on login failure', async () => {
    mockClientLogin.mockRejectedValueOnce(new Error('401: invalid credentials'))

    renderLoginPage()
    await waitForReady()

    await userEvent.type(screen.getByLabelText(/username/i), 'bad')
    await userEvent.type(screen.getByLabelText(/password/i), 'wrong')
    await userEvent.click(screen.getByRole('button', { name: /sign in/i }))

    await waitFor(() => screen.getByRole('alert'))
    // Still on the login page
    expect(screen.getByLabelText(/username/i)).toBeInTheDocument()
  })

  it('disables the sign-in button while a request is in flight', async () => {
    // Use a never-resolving promise to freeze the loading state
    mockClientLogin.mockReturnValueOnce(new Promise(() => {}))

    renderLoginPage()
    await waitForReady()

    await userEvent.type(screen.getByLabelText(/username/i), 'alice')
    await userEvent.type(screen.getByLabelText(/password/i), 'secret')

    // Click the button — immediately after, it should be disabled while loading
    await userEvent.click(screen.getByRole('button', { name: /sign in/i }))

    await waitFor(() =>
      expect(screen.getByRole('button', { name: /signing in/i })).toBeDisabled()
    )
  })
})

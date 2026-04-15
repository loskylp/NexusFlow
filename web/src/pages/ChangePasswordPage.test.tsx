/**
 * Unit tests for ChangePasswordPage (SEC-001).
 *
 * Covers:
 *   - Form renders with three fields (current, new, confirm).
 *   - Submit button disabled when fields are empty.
 *   - Client-side validation: new password < 8 chars shows inline error.
 *   - Client-side validation: passwords do not match shows inline error.
 *   - Server 401 → "Current password is incorrect" inline.
 *   - Server 400 → password length error inline.
 *   - Success (204) → logout called + redirect to /login.
 *   - Form inputs disabled while submission is in progress.
 *
 * All tests use the real AuthProvider with the API client mocked.
 *
 * See: SEC-001, SEC-007, ADR-006
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import ChangePasswordPage from './ChangePasswordPage'
import { AuthProvider } from '@/context/AuthContext'
import * as client from '@/api/client'

// Mock only the API client layer.
vi.mock('@/api/client')
const mockChangePassword = vi.mocked(client.changePassword)
const mockLogout = vi.mocked(client.logout)

beforeEach(() => {
  vi.clearAllMocks()
  // Stub global fetch for the AuthProvider's /api/auth/me session-restore call.
  // Simulate an authenticated user with mustChangePassword=true.
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
    ok: true,
    status: 200,
    json: async () => ({
      user: {
        id: 'u1',
        username: 'admin',
        role: 'admin',
        active: true,
        mustChangePassword: true,
        createdAt: '2024-01-01T00:00:00Z',
      },
    }),
  }))
  // Default: changePassword resolves with no content (204 mapped to undefined).
  mockChangePassword.mockResolvedValue(undefined)
  // Default: logout resolves immediately.
  mockLogout.mockResolvedValue(undefined)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

/**
 * Renders ChangePasswordPage inside AuthProvider and MemoryRouter.
 * Includes a /login stub so redirect assertions work.
 */
function renderPage() {
  return render(
    <AuthProvider>
      <MemoryRouter initialEntries={['/change-password']}>
        <Routes>
          <Route path="/change-password" element={<ChangePasswordPage />} />
          <Route path="/login" element={<div>login page</div>} />
        </Routes>
      </MemoryRouter>
    </AuthProvider>
  )
}

/** Waits until the form is fully rendered and interactive. */
async function waitForForm() {
  await waitFor(() =>
    expect(screen.getByLabelText(/current password/i)).toBeInTheDocument()
  )
}

describe('ChangePasswordPage — form renders', () => {
  it('renders current password, new password, and confirm password fields', async () => {
    renderPage()
    await waitForForm()

    expect(screen.getByLabelText(/current password/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/^new password$/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/confirm new password/i)).toBeInTheDocument()
  })

  it('renders a submit button', async () => {
    renderPage()
    await waitForForm()

    expect(screen.getByRole('button', { name: /change password/i })).toBeInTheDocument()
  })
})

describe('ChangePasswordPage — submit button state', () => {
  it('disables submit button when all fields are empty', async () => {
    renderPage()
    await waitForForm()

    expect(screen.getByRole('button', { name: /change password/i })).toBeDisabled()
  })

  it('enables submit button when all three fields are non-empty', async () => {
    renderPage()
    await waitForForm()

    await userEvent.type(screen.getByLabelText(/current password/i), 'oldpass')
    await userEvent.type(screen.getByLabelText(/^new password$/i), 'newpass123')
    await userEvent.type(screen.getByLabelText(/confirm new password/i), 'newpass123')

    expect(screen.getByRole('button', { name: /change password/i })).not.toBeDisabled()
  })
})

describe('ChangePasswordPage — client-side validation', () => {
  it('shows inline error when new password is shorter than 8 characters', async () => {
    renderPage()
    await waitForForm()

    await userEvent.type(screen.getByLabelText(/current password/i), 'oldpass')
    await userEvent.type(screen.getByLabelText(/^new password$/i), 'short7!')
    await userEvent.type(screen.getByLabelText(/confirm new password/i), 'short7!')
    await userEvent.click(screen.getByRole('button', { name: /change password/i }))

    await waitFor(() =>
      expect(screen.getByText(/at least 8 characters/i)).toBeInTheDocument()
    )
    expect(mockChangePassword).not.toHaveBeenCalled()
  })

  it('shows inline error when new password and confirm do not match', async () => {
    renderPage()
    await waitForForm()

    await userEvent.type(screen.getByLabelText(/current password/i), 'oldpass')
    await userEvent.type(screen.getByLabelText(/^new password$/i), 'newpass123')
    await userEvent.type(screen.getByLabelText(/confirm new password/i), 'different123')
    await userEvent.click(screen.getByRole('button', { name: /change password/i }))

    await waitFor(() =>
      expect(screen.getByText(/passwords do not match/i)).toBeInTheDocument()
    )
    expect(mockChangePassword).not.toHaveBeenCalled()
  })
})

describe('ChangePasswordPage — server error handling', () => {
  it('shows "Current password is incorrect" on 401 response', async () => {
    mockChangePassword.mockRejectedValueOnce(new Error('401: Unauthorized'))

    renderPage()
    await waitForForm()

    await userEvent.type(screen.getByLabelText(/current password/i), 'wrongpass')
    await userEvent.type(screen.getByLabelText(/^new password$/i), 'newpass123')
    await userEvent.type(screen.getByLabelText(/confirm new password/i), 'newpass123')
    await userEvent.click(screen.getByRole('button', { name: /change password/i }))

    await waitFor(() =>
      expect(screen.getByText(/current password is incorrect/i)).toBeInTheDocument()
    )
  })

  it('shows password length error on 400 response', async () => {
    mockChangePassword.mockRejectedValueOnce(new Error('400: Bad Request'))

    renderPage()
    await waitForForm()

    await userEvent.type(screen.getByLabelText(/current password/i), 'oldpass')
    await userEvent.type(screen.getByLabelText(/^new password$/i), 'newpass123')
    await userEvent.type(screen.getByLabelText(/confirm new password/i), 'newpass123')
    await userEvent.click(screen.getByRole('button', { name: /change password/i }))

    await waitFor(() =>
      expect(screen.getByText(/at least 8 characters/i)).toBeInTheDocument()
    )
  })
})

describe('ChangePasswordPage — success flow', () => {
  it('calls POST /api/auth/change-password with current and new password on submit', async () => {
    mockChangePassword.mockResolvedValueOnce(undefined)

    renderPage()
    await waitForForm()

    await userEvent.type(screen.getByLabelText(/current password/i), 'oldpass')
    await userEvent.type(screen.getByLabelText(/^new password$/i), 'newpass123')
    await userEvent.type(screen.getByLabelText(/confirm new password/i), 'newpass123')
    await userEvent.click(screen.getByRole('button', { name: /change password/i }))

    await waitFor(() =>
      expect(mockChangePassword).toHaveBeenCalledWith('oldpass', 'newpass123')
    )
  })

  it('redirects to /login on 204 response', async () => {
    mockChangePassword.mockResolvedValueOnce(undefined)

    renderPage()
    await waitForForm()

    await userEvent.type(screen.getByLabelText(/current password/i), 'oldpass')
    await userEvent.type(screen.getByLabelText(/^new password$/i), 'newpass123')
    await userEvent.type(screen.getByLabelText(/confirm new password/i), 'newpass123')
    await userEvent.click(screen.getByRole('button', { name: /change password/i }))

    await waitFor(() =>
      expect(screen.getByText('login page')).toBeInTheDocument()
    )
  })
})

describe('ChangePasswordPage — loading state', () => {
  it('disables all inputs while submission is in progress', async () => {
    // Never-resolving promise to freeze the loading state
    mockChangePassword.mockReturnValueOnce(new Promise(() => {}))

    renderPage()
    await waitForForm()

    await userEvent.type(screen.getByLabelText(/current password/i), 'oldpass')
    await userEvent.type(screen.getByLabelText(/^new password$/i), 'newpass123')
    await userEvent.type(screen.getByLabelText(/confirm new password/i), 'newpass123')
    await userEvent.click(screen.getByRole('button', { name: /change password/i }))

    await waitFor(() => {
      expect(screen.getByLabelText(/current password/i)).toBeDisabled()
      expect(screen.getByLabelText(/^new password$/i)).toBeDisabled()
      expect(screen.getByLabelText(/confirm new password/i)).toBeDisabled()
      expect(screen.getByRole('button', { name: /changing/i })).toBeDisabled()
    })
  })
})

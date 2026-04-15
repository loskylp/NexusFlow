/**
 * Unit tests for ProtectedRoute.
 * Covers AC-6 (unauthenticated users redirected to /login).
 *
 * See: TASK-019
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import ProtectedRoute from './ProtectedRoute'
import * as AuthContext from '@/context/AuthContext'
import type { User } from '@/types/domain'

vi.mock('@/context/AuthContext')
const mockUseAuth = vi.mocked(AuthContext.useAuth)

function authenticated(role: 'admin' | 'user' = 'admin'): User {
  return { id: 'u1', username: 'alice', role, active: true, mustChangePassword: false, createdAt: '' }
}

function renderProtected(user: User | null, loadingState = false) {
  mockUseAuth.mockReturnValue({
    user,
    isLoading: loadingState,
    login: vi.fn(),
    logout: vi.fn(),
  })

  render(
    <MemoryRouter initialEntries={['/workers']}>
      <Routes>
        <Route path="/login" element={<div>login page</div>} />
        <Route
          path="/workers"
          element={
            <ProtectedRoute>
              <div>workers page</div>
            </ProtectedRoute>
          }
        />
      </Routes>
    </MemoryRouter>
  )
}

beforeEach(() => vi.clearAllMocks())

describe('ProtectedRoute (AC-6)', () => {
  it('renders children when user is authenticated', () => {
    renderProtected(authenticated())
    expect(screen.getByText('workers page')).toBeInTheDocument()
  })

  it('redirects to /login when user is null', () => {
    renderProtected(null)
    expect(screen.getByText('login page')).toBeInTheDocument()
    expect(screen.queryByText('workers page')).not.toBeInTheDocument()
  })

  it('shows nothing (or loading) while auth is loading', () => {
    renderProtected(null, true)
    // During loading we should not redirect yet (avoids flash of /login on refresh)
    expect(screen.queryByText('login page')).not.toBeInTheDocument()
    expect(screen.queryByText('workers page')).not.toBeInTheDocument()
  })
})

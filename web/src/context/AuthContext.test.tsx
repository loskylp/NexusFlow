/**
 * Unit tests for AuthContext and AuthProvider.
 * Covers: initial state, login success/failure, logout, loading state, role-in-context.
 *
 * See: TASK-019, AC-2, AC-3
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import React from 'react'
import { AuthProvider, useAuth } from './AuthContext'
import * as client from '@/api/client'
import type { AuthResponse } from '@/types/domain'

// Isolate from real network
vi.mock('@/api/client')
const mockLogin = vi.mocked(client.login)
const mockLogout = vi.mocked(client.logout)

// Helper component that exercises AuthContext.
// Login and logout errors are swallowed here so tests can assert on state
// without unhandled rejection warnings. The rejection behaviour itself
// is tested in LoginPage tests where the error message is rendered.
function TestConsumer(): React.ReactElement {
  const { user, isLoading, login, logout } = useAuth()
  return (
    <div>
      <span data-testid="loading">{String(isLoading)}</span>
      <span data-testid="username">{user?.username ?? 'null'}</span>
      <span data-testid="role">{user?.role ?? 'null'}</span>
      <button onClick={() => void login('alice', 'pass').catch(() => {})}>login</button>
      <button onClick={() => void logout()}>logout</button>
    </div>
  )
}

function renderWithProvider(): void {
  render(
    <AuthProvider>
      <TestConsumer />
    </AuthProvider>
  )
}

// Default fetch stub: /api/auth/me returns 404 (no prior session).
// Individual tests can override via vi.stubGlobal('fetch', ...).
beforeEach(() => {
  vi.clearAllMocks()
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
    ok: false,
    status: 404,
    json: async () => ({}),
  }))
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('AuthProvider — initial state', () => {
  it('starts with no user; isLoading is true during session check, then false', async () => {
    // The provider performs a GET /api/auth/me on mount (session restore).
    // We stub fetch to return 401 so the check resolves quickly.
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 401, json: async () => ({}) }))
    renderWithProvider()

    // After the async session check resolves, isLoading should be false.
    await waitFor(() =>
      expect(screen.getByTestId('loading').textContent).toBe('false')
    )
    expect(screen.getByTestId('username').textContent).toBe('null')
    vi.unstubAllGlobals()
  })
})

describe('AuthProvider — login()', () => {
  it('sets user in context on successful login', async () => {
    const response: AuthResponse = {
      token: 'tok-1',
      user: { id: 'u1', username: 'alice', role: 'admin', active: true, mustChangePassword: false, createdAt: '' },
    }
    mockLogin.mockResolvedValueOnce(response)

    renderWithProvider()
    await userEvent.click(screen.getByText('login'))

    await waitFor(() =>
      expect(screen.getByTestId('username').textContent).toBe('alice')
    )
    expect(screen.getByTestId('role').textContent).toBe('admin')
  })

  it('leaves user as null when login fails', async () => {
    mockLogin.mockRejectedValueOnce(new Error('401: invalid credentials'))

    renderWithProvider()
    await userEvent.click(screen.getByText('login'))

    // user must remain null after a failed login
    await waitFor(() =>
      expect(screen.getByTestId('username').textContent).toBe('null')
    )
  })

  it('returns user with role "user" for a regular user login', async () => {
    const response: AuthResponse = {
      token: 'tok-2',
      user: { id: 'u2', username: 'bob', role: 'user', active: true, mustChangePassword: false, createdAt: '' },
    }
    mockLogin.mockResolvedValueOnce(response)

    renderWithProvider()
    await userEvent.click(screen.getByText('login'))

    await waitFor(() => expect(screen.getByTestId('role').textContent).toBe('user'))
  })
})

describe('AuthProvider — logout()', () => {
  it('clears user from context after logout', async () => {
    const loginResp: AuthResponse = {
      token: 'tok-3',
      user: { id: 'u3', username: 'charlie', role: 'admin', active: true, mustChangePassword: false, createdAt: '' },
    }
    mockLogin.mockResolvedValueOnce(loginResp)
    mockLogout.mockResolvedValueOnce(undefined)

    renderWithProvider()
    await userEvent.click(screen.getByText('login'))
    await waitFor(() => expect(screen.getByTestId('username').textContent).toBe('charlie'))

    await userEvent.click(screen.getByText('logout'))
    await waitFor(() => expect(screen.getByTestId('username').textContent).toBe('null'))
  })
})

describe('useAuth guard', () => {
  it('throws when called outside an AuthProvider', () => {
    // Suppress React error boundary noise in console
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})
    expect(() => render(<TestConsumer />)).toThrow()
    spy.mockRestore()
  })
})

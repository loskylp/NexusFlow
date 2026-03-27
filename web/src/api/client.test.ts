/**
 * Unit tests for the API client module.
 * Tests the apiFetch base wrapper: credentials, Content-Type, error propagation.
 * Tests login() and logout() request shapes against the backend contract.
 *
 * See: TASK-019, handlers_auth.go
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { login, logout } from './client'

const mockFetch = vi.fn()

beforeEach(() => {
  vi.stubGlobal('fetch', mockFetch)
  mockFetch.mockReset()
})

describe('login()', () => {
  it('POSTs to /api/auth/login with credentials as JSON', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        token: 'tok-abc',
        user: { id: 'u1', username: 'alice', role: 'admin', active: true, createdAt: '' },
      }),
    })

    await login('alice', 'secret')

    expect(mockFetch).toHaveBeenCalledOnce()
    const [url, init] = mockFetch.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/auth/login')
    expect(init.method).toBe('POST')
    expect(init.credentials).toBe('include')
    expect(JSON.parse(init.body as string)).toEqual({ username: 'alice', password: 'secret' })
    expect((init.headers as Record<string, string>)['Content-Type']).toBe('application/json')
  })

  it('returns the AuthResponse on success', async () => {
    const payload = {
      token: 'tok-xyz',
      user: { id: 'u2', username: 'bob', role: 'user', active: true, createdAt: '2026-01-01' },
    }
    mockFetch.mockResolvedValueOnce({ ok: true, json: async () => payload })

    const result = await login('bob', 'pass')
    expect(result).toEqual(payload)
  })

  it('throws on 401 with a readable error message', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 401,
      text: async () => 'invalid credentials',
    })

    await expect(login('bad', 'wrong')).rejects.toThrow('401')
  })

  it('throws on 500 server error', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 500,
      text: async () => 'internal error',
    })

    await expect(login('alice', 'secret')).rejects.toThrow('500')
  })
})

describe('logout()', () => {
  it('POSTs to /api/auth/logout with credentials include', async () => {
    mockFetch.mockResolvedValueOnce({ ok: true, status: 204, json: async () => ({}) })

    await logout()

    expect(mockFetch).toHaveBeenCalledOnce()
    const [url, init] = mockFetch.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/auth/logout')
    expect(init.method).toBe('POST')
    expect(init.credentials).toBe('include')
  })
})

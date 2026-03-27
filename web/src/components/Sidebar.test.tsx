/**
 * Unit tests for Sidebar component.
 * Covers AC-4 (sidebar visible on authenticated views with correct items),
 * AC-5 (demo nav items hidden for User role).
 *
 * See: TASK-019, ux-spec.md — Role-Based Visibility Rules
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import Sidebar from './Sidebar'
import * as AuthContext from '@/context/AuthContext'
import type { User } from '@/types/domain'

vi.mock('@/context/AuthContext')
const mockUseAuth = vi.mocked(AuthContext.useAuth)

function adminUser(): User {
  return { id: 'u1', username: 'alice', role: 'admin', active: true, createdAt: '' }
}
function regularUser(): User {
  return { id: 'u2', username: 'bob', role: 'user', active: true, createdAt: '' }
}

function makeAuthValue(user: User) {
  return { user, isLoading: false, login: vi.fn(), logout: vi.fn() }
}

function renderSidebar(user: User) {
  mockUseAuth.mockReturnValue(makeAuthValue(user))
  render(
    <MemoryRouter>
      <Sidebar />
    </MemoryRouter>
  )
}

beforeEach(() => vi.clearAllMocks())

describe('Sidebar — primary nav items (AC-4)', () => {
  it('renders Pipeline Builder nav link for admin', () => {
    renderSidebar(adminUser())
    expect(screen.getByRole('link', { name: /pipeline builder/i })).toBeInTheDocument()
  })

  it('renders Worker Fleet nav link for admin', () => {
    renderSidebar(adminUser())
    expect(screen.getByRole('link', { name: /worker fleet/i })).toBeInTheDocument()
  })

  it('renders Task Feed nav link for admin', () => {
    renderSidebar(adminUser())
    expect(screen.getByRole('link', { name: /task feed/i })).toBeInTheDocument()
  })

  it('renders Log Streamer nav link for admin', () => {
    renderSidebar(adminUser())
    expect(screen.getByRole('link', { name: /log streamer/i })).toBeInTheDocument()
  })

  it('renders all primary nav links for regular user', () => {
    renderSidebar(regularUser())
    expect(screen.getByRole('link', { name: /pipeline builder/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /worker fleet/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /task feed/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /log streamer/i })).toBeInTheDocument()
  })

  it('renders logout button', () => {
    renderSidebar(adminUser())
    expect(screen.getByRole('button', { name: /log out|logout/i })).toBeInTheDocument()
  })
})

describe('Sidebar — demo nav visibility (AC-5)', () => {
  it('shows Sink Inspector for admin', () => {
    renderSidebar(adminUser())
    expect(screen.getByRole('link', { name: /sink inspector/i })).toBeInTheDocument()
  })

  it('shows Chaos Controller for admin', () => {
    renderSidebar(adminUser())
    expect(screen.getByRole('link', { name: /chaos controller/i })).toBeInTheDocument()
  })

  it('hides Sink Inspector for regular user (AC-5)', () => {
    renderSidebar(regularUser())
    expect(screen.queryByRole('link', { name: /sink inspector/i })).not.toBeInTheDocument()
  })

  it('hides Chaos Controller for regular user (AC-5)', () => {
    renderSidebar(regularUser())
    expect(screen.queryByRole('link', { name: /chaos controller/i })).not.toBeInTheDocument()
  })
})

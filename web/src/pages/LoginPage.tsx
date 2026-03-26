/**
 * LoginPage — the unauthenticated entry point to NexusFlow.
 * Renders the login form per the UX spec (process/designer/ux-spec.md, Login Screen section).
 *
 * Layout: centered card on full-height slate-900 background.
 * Form fields: Username, Password (per DESIGN.md Form Inputs spec).
 * Error handling: inline error message below the form on 401.
 * On success: navigate to /workers (Admin) or /tasks (User).
 *
 * See: TASK-019
 */

import React, { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '@/hooks/useAuth'

function LoginPage(): React.ReactElement {
  const { login, isLoading } = useAuth()
  const navigate = useNavigate()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)

  // TODO: Implement full LoginPage in TASK-019
  // Requirements:
  //   - NexusFlow logo / wordmark at top of card
  //   - Username input (label: "USERNAME", IBM Plex Sans 12px uppercase)
  //   - Password input (type="password")
  //   - "Log In" primary button (full-width, indigo-500)
  //   - Inline error below button on 401 ("Invalid username or password")
  //   - Loading state: button shows spinner, inputs disabled
  //   - On success: useAuth().login stores user; navigate based on role

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)
    try {
      await login(username, password)
      // TODO: Navigate based on role in TASK-019
      navigate('/workers')
    } catch {
      setError('Invalid username or password')
    }
  }

  return (
    <div>
      {/* TODO: Replace with styled LoginPage in TASK-019 */}
      <form onSubmit={handleSubmit}>
        <input
          type="text"
          value={username}
          onChange={e => setUsername(e.target.value)}
          placeholder="Username"
          disabled={isLoading}
        />
        <input
          type="password"
          value={password}
          onChange={e => setPassword(e.target.value)}
          placeholder="Password"
          disabled={isLoading}
        />
        {error && <p>{error}</p>}
        <button type="submit" disabled={isLoading}>
          {isLoading ? 'Logging in...' : 'Log In'}
        </button>
      </form>
    </div>
  )
}

export default LoginPage

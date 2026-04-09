/**
 * App — root component. Sets up routing and auth context.
 *
 * Route structure:
 *   /login                  — Login page (public)
 *   /                       — Protected: redirect to /workers (Admin) or /tasks (User)
 *   /workers                — Worker Fleet Dashboard (protected, all roles)
 *   /tasks                  — Task Feed (protected, all roles; placeholder for Cycle 2)
 *   /tasks/logs             — Log Streamer (protected; placeholder for Cycle 3)
 *   /pipelines              — Pipeline Builder (protected; placeholder for Cycle 3)
 *   /demo/sink-inspector    — Sink Inspector (protected, Admin; placeholder for Cycle 4)
 *   /demo/chaos             — Chaos Controller (protected, Admin; placeholder for Cycle 4)
 *   *                       — 404 NotFoundPage
 *
 * All protected routes are wrapped in ProtectedRoute (redirects to /login when unauthenticated)
 * and Layout (sidebar + main content area).
 *
 * Uses createBrowserRouter / RouterProvider (data router) to support hooks such as
 * useBlocker that require a data router context. BrowserRouter + Routes does not
 * provide this context and causes a runtime crash on pages that call useBlocker.
 *
 * See: TASK-019
 */

import React from 'react'
import { createBrowserRouter, RouterProvider, Navigate } from 'react-router-dom'
import { AuthProvider } from '@/context/AuthContext'
import { useAuth } from '@/context/AuthContext'
import ProtectedRoute from '@/components/ProtectedRoute'
import Layout from '@/components/Layout'
import LoginPage from '@/pages/LoginPage'
import WorkerFleetDashboard from '@/pages/WorkerFleetDashboard'
import TaskFeedPage from '@/pages/TaskFeedPage'
import PipelineManagerPage from '@/pages/PipelineManagerPage'
import LogStreamerPage from '@/pages/LogStreamerPage'
import SinkInspectorPage from '@/pages/SinkInspectorPage'
import ChaosControllerPage from '@/pages/ChaosControllerPage'
import NotFoundPage from '@/pages/NotFoundPage'

/**
 * RootRedirect resolves the default route (/) to the role-appropriate landing page.
 * Admin lands on /workers; User lands on /tasks (per UX spec navigation design decision 4).
 */
function RootRedirect(): React.ReactElement {
  const { user } = useAuth()
  const destination = user?.role === 'admin' ? '/workers' : '/tasks'
  return <Navigate to={destination} replace />
}

/**
 * router is the data router instance for the application. Defined at module scope
 * so it is created once and stable across re-renders. createBrowserRouter provides
 * the data router context required by hooks such as useBlocker.
 */
const router = createBrowserRouter([
  // Public route
  { path: '/login', element: <LoginPage /> },

  // Protected routes — all wrapped with auth guard and sidebar layout
  {
    path: '/',
    element: (
      <ProtectedRoute>
        <Layout>
          <RootRedirect />
        </Layout>
      </ProtectedRoute>
    ),
  },
  {
    path: '/workers',
    element: (
      <ProtectedRoute>
        <Layout>
          <WorkerFleetDashboard />
        </Layout>
      </ProtectedRoute>
    ),
  },
  {
    path: '/tasks',
    element: (
      <ProtectedRoute>
        <Layout>
          <TaskFeedPage />
        </Layout>
      </ProtectedRoute>
    ),
  },
  {
    path: '/tasks/logs',
    element: (
      <ProtectedRoute>
        <Layout>
          <LogStreamerPage />
        </Layout>
      </ProtectedRoute>
    ),
  },
  {
    path: '/pipelines',
    element: (
      <ProtectedRoute>
        <Layout>
          <PipelineManagerPage />
        </Layout>
      </ProtectedRoute>
    ),
  },
  {
    path: '/demo/sink-inspector',
    element: (
      <ProtectedRoute>
        <Layout>
          <SinkInspectorPage />
        </Layout>
      </ProtectedRoute>
    ),
  },
  {
    path: '/demo/chaos',
    element: (
      <ProtectedRoute>
        <Layout>
          <ChaosControllerPage />
        </Layout>
      </ProtectedRoute>
    ),
  },

  // 404
  { path: '*', element: <NotFoundPage /> },
])

function App(): React.ReactElement {
  return (
    <AuthProvider>
      <RouterProvider router={router} />
    </AuthProvider>
  )
}

export default App

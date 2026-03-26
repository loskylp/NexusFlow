/**
 * App — root component. Sets up routing and auth context.
 * Route structure:
 *   /login                  — Login page (public)
 *   /                       — Protected: redirect to /workers (Admin) or /tasks (User)
 *   /workers                — Worker Fleet Dashboard (Admin; TASK-020)
 *   /tasks                  — Task Feed (Cycle 2)
 *   /tasks/:id/logs         — Log Streamer (Cycle 3)
 *   /pipelines              — Pipeline management (Cycle 3)
 *   /pipelines/new          — Pipeline Builder (Cycle 3)
 *   /sink/:taskId           — Sink Inspector (Cycle 4, demo)
 *   /chaos                  — Chaos Controller (Cycle 4, demo)
 *
 * See: TASK-019
 */

import React from 'react'
import { BrowserRouter, Route, Routes, Navigate } from 'react-router-dom'
import { AuthProvider } from '@/context/AuthContext'
import LoginPage from '@/pages/LoginPage'
import WorkerFleetDashboard from '@/pages/WorkerFleetDashboard'
import NotFoundPage from '@/pages/NotFoundPage'

// TODO: Implement full routing and ProtectedRoute wrapper in TASK-019

function App(): React.ReactElement {
  return (
    <AuthProvider>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/workers" element={<WorkerFleetDashboard />} />
          <Route path="/" element={<Navigate to="/workers" replace />} />
          <Route path="*" element={<NotFoundPage />} />
        </Routes>
      </BrowserRouter>
    </AuthProvider>
  )
}

export default App

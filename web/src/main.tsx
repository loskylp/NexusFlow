/**
 * main.tsx — React application entry point.
 * Mounts the App component into the DOM.
 * See: TASK-019
 */

import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'
import './styles/globals.css'

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
)

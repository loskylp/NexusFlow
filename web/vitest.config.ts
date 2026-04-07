/**
 * Vitest configuration for the NexusFlow React frontend.
 * Uses jsdom for DOM simulation and re-uses the Vite alias resolution (@/ -> src/).
 * See: TASK-019
 *
 * Verifier tests (integration + acceptance) live in tests/ at the project root.
 * They are included here via the include patterns and resolved against web/node_modules
 * using the alias configuration.
 */
import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import { resolve } from 'path'

const webDir = __dirname
const projectRoot = resolve(__dirname, '..')
const nodeModules = resolve(webDir, 'node_modules')

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': resolve(webDir, 'src'),
      // Resolve bare module specifiers from web/node_modules for tests that
      // live outside web/ (Verifier integration and acceptance tests).
      'react': resolve(nodeModules, 'react'),
      'react-dom': resolve(nodeModules, 'react-dom'),
      'react-dom/client': resolve(nodeModules, 'react-dom/client'),
      'react-router-dom': resolve(nodeModules, 'react-router-dom'),
      '@testing-library/react': resolve(nodeModules, '@testing-library/react'),
      '@testing-library/jest-dom': resolve(nodeModules, '@testing-library/jest-dom'),
      '@testing-library/user-event': resolve(nodeModules, '@testing-library/user-event'),
      'vitest': resolve(nodeModules, 'vitest'),
    },
  },
  server: {
    fs: {
      // Allow vitest to access test files in the project-level tests/ directory
      // (Verifier acceptance and integration tests live outside web/).
      allow: [projectRoot, webDir],
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: [resolve(webDir, 'src/test/setup.ts')],
    include: [
      'src/**/*.test.{ts,tsx}',
      '../tests/integration/**/*.test.{ts,tsx}',
      '../tests/acceptance/**/*.{test,spec}.{ts,tsx}',
    ],
    deps: {
      moduleDirectories: [nodeModules, 'node_modules'],
      interopDefault: true,
    },
  },
})

/**
 * useAuth re-exports the hook from AuthContext for ergonomic imports.
 * Import from here rather than directly from context to keep import paths stable.
 *
 * See: TASK-019
 */
export { useAuth } from '@/context/AuthContext'

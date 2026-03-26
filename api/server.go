// Package api implements the NexusFlow API server.
// HTTP router: chi (lightweight, idiomatic Go routing).
// All routes are mounted under /api. The web frontend is served separately by nginx.
// Auth middleware (ADR-006) is applied to all routes except POST /api/auth/login and GET /api/health.
// See: ADR-004, ADR-006, ADR-007, TASK-003, TASK-005, TASK-013, TASK-015, TASK-025
package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/nxlabs/nexusflow/internal/config"
	"github.com/nxlabs/nexusflow/internal/db"
	"github.com/nxlabs/nexusflow/internal/queue"
	"github.com/nxlabs/nexusflow/internal/sse"
	"github.com/redis/go-redis/v9"
)

// Server holds all dependencies needed to serve the NexusFlow REST API.
// Dependencies are injected at construction time; no global state.
type Server struct {
	cfg       *config.Config
	pool      *db.Pool
	redis     *redis.Client
	users     db.UserRepository
	tasks     db.TaskRepository
	pipelines db.PipelineRepository
	workers   db.WorkerRepository
	producer  queue.Producer
	sessions  queue.SessionStore
	broker    sse.Broker
}

// NewServer constructs the API Server with all required dependencies.
//
// Args:
//
//	cfg:       Loaded runtime configuration.
//	pool:      PostgreSQL connection pool for health checks.
//	redis:     go-redis client for health checks.
//	users:     UserRepository backed by PostgreSQL.
//	tasks:     TaskRepository backed by PostgreSQL.
//	pipelines: PipelineRepository backed by PostgreSQL.
//	workers:   WorkerRepository backed by PostgreSQL.
//	producer:  Queue Producer for enqueuing tasks into Redis Streams.
//	sessions:  SessionStore backed by Redis for auth middleware.
//	broker:    SSE Broker for real-time event distribution.
//
// Preconditions:
//   - All arguments are non-nil and their underlying connections are open.
func NewServer(
	cfg *config.Config,
	pool *db.Pool,
	redisClient *redis.Client,
	users db.UserRepository,
	tasks db.TaskRepository,
	pipelines db.PipelineRepository,
	workers db.WorkerRepository,
	producer queue.Producer,
	sessions queue.SessionStore,
	broker sse.Broker,
) *Server {
	return &Server{
		cfg:       cfg,
		pool:      pool,
		redis:     redisClient,
		users:     users,
		tasks:     tasks,
		pipelines: pipelines,
		workers:   workers,
		producer:  producer,
		sessions:  sessions,
		broker:    broker,
	}
}

// Handler builds and returns the http.Handler for the API server.
// Registers all routes and applies middleware.
// Called once at startup; the returned handler is passed to http.ListenAndServe.
//
// Route map:
//
//	POST   /api/auth/login             — AuthHandler.Login       (TASK-003)
//	POST   /api/auth/logout            — AuthHandler.Logout      (TASK-003)
//	GET    /api/health                 — HealthHandler.Health    (TASK-001)
//	POST   /api/tasks                  — TaskHandler.Submit      (TASK-005)
//	GET    /api/tasks                  — TaskHandler.List        (TASK-008, Cycle 2)
//	GET    /api/tasks/{id}             — TaskHandler.Get         (TASK-008, Cycle 2)
//	POST   /api/tasks/{id}/cancel      — TaskHandler.Cancel      (TASK-012, Cycle 2)
//	GET    /api/pipelines              — PipelineHandler.List    (TASK-013)
//	POST   /api/pipelines              — PipelineHandler.Create  (TASK-013)
//	GET    /api/pipelines/{id}         — PipelineHandler.Get     (TASK-013)
//	PUT    /api/pipelines/{id}         — PipelineHandler.Update  (TASK-013)
//	DELETE /api/pipelines/{id}         — PipelineHandler.Delete  (TASK-013)
//	GET    /api/workers                — WorkerHandler.List      (TASK-025)
//	GET    /events/tasks               — SSEHandler.Tasks        (TASK-015)
//	GET    /events/workers             — SSEHandler.Workers      (TASK-015)
//	GET    /events/tasks/{id}/logs     — SSEHandler.Logs         (TASK-015)
//	GET    /events/sink/{taskId}       — SSEHandler.Sink         (TASK-015)
//
// Only the health endpoint is fully implemented in TASK-001.
// All other route handlers are scaffolded stubs implemented in later tasks.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	// Health check is unauthenticated (ADR-005 — monitored by Uptime Kuma).
	health := &HealthHandler{server: s}
	r.Get("/api/health", health.Health)

	// --- Auth routes (TASK-003) ---
	authH := &AuthHandler{server: s}
	r.Post("/api/auth/login", authH.Login)
	r.Post("/api/auth/logout", authH.Logout)

	// --- Protected routes ---
	// Auth middleware is wired in TASK-003. Handlers below panic until their task is implemented.
	taskH := &TaskHandler{server: s}
	r.Post("/api/tasks", taskH.Submit)
	r.Get("/api/tasks", taskH.List)
	r.Get("/api/tasks/{id}", taskH.Get)
	r.Post("/api/tasks/{id}/cancel", taskH.Cancel)

	pipelineH := &PipelineHandler{server: s}
	r.Post("/api/pipelines", pipelineH.Create)
	r.Get("/api/pipelines", pipelineH.List)
	r.Get("/api/pipelines/{id}", pipelineH.Get)
	r.Put("/api/pipelines/{id}", pipelineH.Update)
	r.Delete("/api/pipelines/{id}", pipelineH.Delete)

	workerH := &WorkerHandler{server: s}
	r.Get("/api/workers", workerH.List)

	// --- SSE routes (TASK-015) ---
	sseH := &SSEHandler{server: s}
	r.Get("/events/tasks", sseH.Tasks)
	r.Get("/events/workers", sseH.Workers)
	r.Get("/events/tasks/{id}/logs", sseH.Logs)
	r.Get("/events/sink/{taskId}", sseH.Sink)

	return r
}

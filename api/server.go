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
	"github.com/nxlabs/nexusflow/internal/auth"
	"github.com/nxlabs/nexusflow/internal/config"
	"github.com/nxlabs/nexusflow/internal/db"
	"github.com/nxlabs/nexusflow/internal/models"
	"github.com/nxlabs/nexusflow/internal/queue"
	"github.com/nxlabs/nexusflow/internal/sse"
	"github.com/redis/go-redis/v9"
)

// Server holds all dependencies needed to serve the NexusFlow REST API.
// Dependencies are injected at construction time; no global state.
type Server struct {
	cfg           *config.Config
	pool          *db.Pool
	redis         *redis.Client
	users         db.UserRepository
	tasks         db.TaskRepository
	taskLogs      db.TaskLogRepository
	pipelines     db.PipelineRepository
	workers       db.WorkerRepository
	chains        db.ChainRepository
	producer      queue.Producer
	sessions      queue.SessionStore
	broker        sse.Broker
	cancellations queue.CancellationStore
}

// NewServer constructs the API Server with all required dependencies.
//
// Args:
//
//	cfg:           Loaded runtime configuration.
//	pool:          PostgreSQL connection pool for health checks.
//	redis:         go-redis client for health checks.
//	users:         UserRepository backed by PostgreSQL.
//	tasks:         TaskRepository backed by PostgreSQL.
//	taskLogs:      TaskLogRepository for cold log storage (TASK-016).
//	pipelines:     PipelineRepository backed by PostgreSQL.
//	workers:       WorkerRepository backed by PostgreSQL.
//	chains:        ChainRepository backed by PostgreSQL (TASK-014).
//	producer:      Queue Producer for enqueuing tasks into Redis Streams.
//	sessions:      SessionStore backed by Redis for auth middleware.
//	broker:        SSE Broker for real-time event distribution.
//	cancellations: CancellationStore for setting cancel flags on running tasks.
//
// Preconditions:
//   - All arguments are non-nil and their underlying connections are open.
func NewServer(
	cfg *config.Config,
	pool *db.Pool,
	redisClient *redis.Client,
	users db.UserRepository,
	tasks db.TaskRepository,
	taskLogs db.TaskLogRepository,
	pipelines db.PipelineRepository,
	workers db.WorkerRepository,
	chains db.ChainRepository,
	producer queue.Producer,
	sessions queue.SessionStore,
	broker sse.Broker,
	cancellations queue.CancellationStore,
) *Server {
	return &Server{
		cfg:           cfg,
		pool:          pool,
		redis:         redisClient,
		users:         users,
		tasks:         tasks,
		taskLogs:      taskLogs,
		pipelines:     pipelines,
		workers:       workers,
		chains:        chains,
		producer:      producer,
		sessions:      sessions,
		broker:        broker,
		cancellations: cancellations,
	}
}

// Handler builds and returns the http.Handler for the API server.
// Registers all routes and applies middleware.
// Called once at startup; the returned handler is passed to http.ListenAndServe.
//
// Route map:
//
//	POST   /api/auth/login             — AuthHandler.Login       (TASK-003) — public
//	POST   /api/auth/logout            — AuthHandler.Logout      (TASK-003) — authenticated
//	GET    /api/health                 — HealthHandler.Health    (TASK-001) — public
//	GET    /api/openapi.json           — OpenAPIHandler.ServeSpec (TASK-027) — public
//	POST   /api/tasks                  — TaskHandler.Submit      (TASK-005) — authenticated
//	GET    /api/tasks                  — TaskHandler.List        (TASK-008, Cycle 2) — authenticated
//	GET    /api/tasks/{id}             — TaskHandler.Get         (TASK-008, Cycle 2) — authenticated
//	POST   /api/tasks/{id}/cancel      — TaskHandler.Cancel      (TASK-012, Cycle 2) — authenticated
//	GET    /api/tasks/{id}/logs        — LogHandler.GetLogs      (TASK-016) — authenticated
//	GET    /api/pipelines              — PipelineHandler.List    (TASK-013) — authenticated
//	POST   /api/pipelines              — PipelineHandler.Create  (TASK-013) — authenticated
//	GET    /api/pipelines/{id}         — PipelineHandler.Get     (TASK-013) — authenticated
//	PUT    /api/pipelines/{id}         — PipelineHandler.Update  (TASK-013) — authenticated
//	DELETE /api/pipelines/{id}         — PipelineHandler.Delete  (TASK-013) — authenticated
//	GET    /api/workers                — WorkerHandler.List      (TASK-025) — authenticated
//	POST   /api/chains                 — ChainHandler.Create     (TASK-014) — authenticated
//	GET    /api/chains/{id}            — ChainHandler.Get        (TASK-014) — authenticated
//	POST   /api/users                  — UserHandler.CreateUser  (TASK-017) — admin
//	GET    /api/users                  — UserHandler.ListUsers   (TASK-017) — admin
//	PUT    /api/users/{id}/deactivate  — UserHandler.DeactivateUser (TASK-017) — admin
//	GET    /events/tasks               — SSEHandler.Tasks        (TASK-015) — authenticated
//	GET    /events/workers             — SSEHandler.Workers      (TASK-015) — authenticated
//	GET    /events/tasks/{id}/logs     — SSEHandler.Logs         (TASK-015) — authenticated
//	GET    /events/sink/{taskId}       — SSEHandler.Sink         (TASK-015) — authenticated
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	// Health check is unauthenticated (ADR-005 — monitored by Uptime Kuma).
	health := &HealthHandler{server: s}
	r.Get("/api/health", health.Health)

	// OpenAPI spec is unauthenticated (TASK-027 — used by swagger-ui and openapi-typescript).
	openAPIH := NewOpenAPIHandler(MustLoadOpenAPISpecJSON())
	r.Get("/api/openapi.json", openAPIH.ServeSpec)

	// Login is public — no auth middleware.
	authH := &AuthHandler{server: s}
	r.Post("/api/auth/login", authH.Login)

	// All routes below require a valid session token (ADR-006).
	// The group applies auth.Middleware to every route registered within it.
	r.Group(func(protected chi.Router) {
		if s.sessions != nil {
			protected.Use(auth.Middleware(s.sessions))
		}

		protected.Post("/api/auth/logout", authH.Logout)

		taskH := &TaskHandler{server: s}
		protected.Post("/api/tasks", taskH.Submit)
		protected.Get("/api/tasks", taskH.List)
		protected.Get("/api/tasks/{id}", taskH.Get)
		protected.Post("/api/tasks/{id}/cancel", taskH.Cancel)

		// Log history endpoint (TASK-016).
		logH := &LogHandler{server: s}
		protected.Get("/api/tasks/{id}/logs", logH.GetLogs)

		pipelineH := &PipelineHandler{server: s}
		protected.Post("/api/pipelines", pipelineH.Create)
		protected.Get("/api/pipelines", pipelineH.List)
		protected.Get("/api/pipelines/{id}", pipelineH.Get)
		protected.Put("/api/pipelines/{id}", pipelineH.Update)
		protected.Delete("/api/pipelines/{id}", pipelineH.Delete)

		workerH := &WorkerHandler{server: s}
		protected.Get("/api/workers", workerH.List)

		// User management routes (TASK-017): admin-only sub-group.
		userH := &UserHandler{server: s}
		protected.Group(func(admin chi.Router) {
			admin.Use(auth.RequireRole(models.RoleAdmin))
			admin.Post("/api/users", userH.CreateUser)
			admin.Get("/api/users", userH.ListUsers)
			admin.Put("/api/users/{id}/deactivate", userH.DeactivateUser)
		})

		// Chain routes (TASK-014).
		chainH := &ChainHandler{server: s}
		protected.Post("/api/chains", chainH.Create)
		protected.Get("/api/chains/{id}", chainH.Get)

		// SSE routes (TASK-015).
		sseH := &SSEHandler{server: s}
		protected.Get("/events/tasks", sseH.Tasks)
		protected.Get("/events/workers", sseH.Workers)
		protected.Get("/events/tasks/{id}/logs", sseH.Logs)
		protected.Get("/events/sink/{taskId}", sseH.Sink)
	})

	return r
}

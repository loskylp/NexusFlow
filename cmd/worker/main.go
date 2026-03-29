// Command worker is the entry point for the NexusFlow Worker process.
// Workers are stateless compute nodes; multiple instances can run simultaneously
// and are scaled via `docker compose up --scale worker=N`.
//
// Startup sequence:
//  1. Load config (DATABASE_URL, REDIS_URL, WORKER_TAGS, WORKER_ID required)
//  2. Connect to PostgreSQL (run migrations)
//  3. Connect to Redis (verify with PING)
//  4. Build WorkerRepository, TaskRepository, PipelineRepository, HeartbeatStore, Consumer
//  5. Build ConnectorRegistry and register demo connectors (TASK-042 wires real connectors)
//  6. Construct Worker and call Run (blocks until SIGTERM/SIGINT)
//  7. Graceful shutdown: Run marks worker as "down" before returning
//
// WORKER_ID defaults to a UUID generated on startup if not set via environment.
// WORKER_TAGS is a comma-separated list (e.g. "etl,report"). Defaults to "demo".
//
// See: ADR-001, ADR-002, ADR-004, ADR-005, TASK-006, TASK-007, TASK-042
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/config"
	"github.com/nxlabs/nexusflow/internal/db"
	"github.com/nxlabs/nexusflow/internal/queue"
	"github.com/redis/go-redis/v9"

	workerPkg "github.com/nxlabs/nexusflow/worker"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("worker: starting")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("worker: configuration error: %v", err)
	}

	// Assign a stable worker ID if not set via environment.
	if cfg.WorkerID == "" {
		cfg.WorkerID = generateWorkerID()
	}

	// Default tags to "demo" when no tags are configured.
	if len(cfg.WorkerTags) == 0 {
		cfg.WorkerTags = []string{"demo"}
	}

	log.Printf("worker: tags=%v id=%q env=%q", cfg.WorkerTags, cfg.WorkerID, cfg.Env)

	// Connect to PostgreSQL. Migrations run automatically on successful connection.
	startupCtx, startupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer startupCancel()

	pool, err := db.New(startupCtx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("worker: postgres: %v", err)
	}
	defer pool.Close()
	log.Printf("worker: postgres connected")

	// Connect Redis.
	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Fatalf("worker: invalid REDIS_URL %q: %v", cfg.RedisURL, err)
	}
	redisClient := redis.NewClient(redisOpts)
	defer func() { _ = redisClient.Close() }()

	if err := redisClient.Ping(startupCtx).Err(); err != nil {
		log.Fatalf("worker: redis not reachable: %v", err)
	}
	log.Printf("worker: redis connected at %s", cfg.RedisURL)

	// Build dependencies wired in TASK-007.
	workerRepo := db.NewPgWorkerRepository(pool)
	taskRepo := db.NewPgTaskRepository(pool)
	pipelineRepo := db.NewPgPipelineRepository(pool)
	redisQueue := queue.NewRedisQueue(redisClient)

	// Build connector registry and register connectors.
	// Demo connectors (type "demo") provide the walking skeleton for all three phases.
	// Atomic sink connectors (types "database", "s3", "file") provide production-grade
	// atomicity and idempotency at the Sink boundary (TASK-018, ADR-003, ADR-009).
	connectorRegistry := workerPkg.NewDefaultConnectorRegistry()
	workerPkg.RegisterDemoConnectors(connectorRegistry)
	workerPkg.RegisterAtomicSinkConnectors(connectorRegistry)

	// Construct the worker.
	// broker is nil until TASK-015 (SSE infrastructure) is implemented.
	// When broker is nil, the Worker skips event publication silently.
	w := workerPkg.NewWorkerWithPipelines(
		cfg,
		taskRepo,
		workerRepo,
		pipelineRepo,
		redisQueue, // Consumer
		redisQueue, // HeartbeatStore
		nil,        // Broker — wired in TASK-015
		connectorRegistry,
		redisQueue, // CancellationStore — TASK-012
	)

	// Run blocks until SIGTERM/SIGINT.
	runCtx, runCancel := context.WithCancel(context.Background())

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	errCh := make(chan error, 1)
	go func() {
		errCh <- w.Run(runCtx)
	}()

	select {
	case sig := <-quit:
		log.Printf("worker: received signal %s — shutting down", sig)
		runCancel()
		if err := <-errCh; err != nil {
			log.Printf("worker: Run returned: %v", err)
		}
	case err := <-errCh:
		runCancel()
		if err != nil {
			log.Fatalf("worker: Run failed: %v", err)
		}
	}

	log.Printf("worker: stopped cleanly")
}

// generateWorkerID produces a unique worker identifier combining the hostname and a UUID suffix.
// Falls back to a pure UUID when the hostname is unavailable.
// The format "hostname-uuid[:8]" provides a human-readable prefix while still guaranteeing
// uniqueness in environments where multiple workers run on the same host.
func generateWorkerID() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		return uuid.New().String()
	}
	suffix := uuid.New().String()[:8]
	return fmt.Sprintf("%s-%s", hostname, suffix)
}

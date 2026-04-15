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
	"net/url"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/config"
	"github.com/nxlabs/nexusflow/internal/db"
	"github.com/nxlabs/nexusflow/internal/queue"
	"github.com/nxlabs/nexusflow/internal/sse"
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
	chainRepo := db.NewPgChainRepository(pool)
	redisQueue := queue.NewRedisQueue(redisClient)

	// Build connector registry and register connectors.
	// Demo connectors (type "demo") provide the walking skeleton for all three phases.
	// Atomic sink connectors (types "database", "s3", "file") provide production-grade
	// atomicity and idempotency at the Sink boundary (TASK-018, ADR-003, ADR-009).
	connectorRegistry := workerPkg.NewDefaultConnectorRegistry()
	workerPkg.RegisterDemoConnectors(connectorRegistry)
	workerPkg.RegisterAtomicSinkConnectors(connectorRegistry)

	// Register MinIO connectors when MINIO_ENDPOINT is configured (TASK-030).
	// In the demo Docker Compose profile, MINIO_ENDPOINT is set to http://minio:9000.
	// Workers started without MinIO (e.g. unit-test environments) skip this step and
	// log a warning so the omission is visible in the startup logs.
	if err := registerMinIOConnectors(connectorRegistry); err != nil {
		log.Fatalf("worker: MinIO connector registration failed: %v", err)
	}

	// Construct the chain trigger (TASK-014, ADR-003).
	// WorkerChainEnqueuer creates and enqueues downstream tasks when a chained task completes.
	// RedisQueue.SetNX provides the SET-NX idempotency guard per ADR-003.
	chainEnqueuer := workerPkg.NewWorkerChainEnqueuer(taskRepo, redisQueue, cfg.WorkerTags)
	chainTrigger := workerPkg.NewChainTrigger(chainRepo, chainEnqueuer, redisQueue)

	// Construct the SSE broker for task event publication and log line fan-out (TASK-015, TASK-016).
	// The broker connects to the same Redis instance and publishes to events:tasks:* and events:logs:*.
	runCtxForBroker, runCancelForBroker := context.WithCancel(context.Background())
	sseBroker := sse.NewRedisBroker(redisClient)
	go func() {
		if err := sseBroker.Start(runCtxForBroker); err != nil {
			log.Printf("worker: SSE broker exited: %v", err)
		}
	}()

	// Construct the LogPublisher (TASK-016).
	// Writes log lines to Redis Streams (hot storage) and publishes to events:logs:{taskId} for SSE.
	logPublisher := workerPkg.NewRedisLogPublisher(redisClient, sseBroker)

	// Construct the worker.
	w := workerPkg.NewWorkerWithPipelines(
		cfg,
		taskRepo,
		workerRepo,
		pipelineRepo,
		redisQueue, // Consumer
		redisQueue, // HeartbeatStore
		sseBroker,  // TaskEventBroker — wired in TASK-015
		connectorRegistry,
		redisQueue, // CancellationStore — TASK-012
	).WithChainTrigger(chainTrigger).WithLogPublisher(logPublisher)

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
		runCancelForBroker() // stop SSE broker goroutine
		if err := <-errCh; err != nil {
			log.Printf("worker: Run returned: %v", err)
		}
	case err := <-errCh:
		runCancel()
		runCancelForBroker()
		if err != nil {
			log.Fatalf("worker: Run failed: %v", err)
		}
	}

	log.Printf("worker: stopped cleanly")
}

// registerMinIOConnectors wires the MinIO DataSource and Sink connectors into reg using
// the MINIO_ENDPOINT, MINIO_ROOT_USER, and MINIO_ROOT_PASSWORD environment variables.
//
// When MINIO_ENDPOINT is empty the function logs a warning and returns nil — the worker
// starts without MinIO support. This allows the worker to run in environments where
// MinIO is not available (e.g. CI, non-demo deployments).
//
// The endpoint scheme (http:// or https://) controls the useSSL flag: http → false,
// https → true. The host:port portion is passed to the minio-go client.
//
// Returns a non-nil error only when MINIO_ENDPOINT is set but the client cannot be
// initialised (e.g. malformed URL, missing credentials).
//
// See: DEMO-001, TASK-030
func registerMinIOConnectors(reg *workerPkg.DefaultConnectorRegistry) error {
	rawEndpoint := os.Getenv("MINIO_ENDPOINT")
	if rawEndpoint == "" {
		log.Printf("worker: MINIO_ENDPOINT not set — MinIO connectors not registered")
		return nil
	}

	accessKey := os.Getenv("MINIO_ROOT_USER")
	secretKey := os.Getenv("MINIO_ROOT_PASSWORD")

	// Derive the host:port and useSSL from the full endpoint URL.
	// minio-go expects the endpoint without scheme (e.g. "minio:9000").
	u, err := url.Parse(rawEndpoint)
	if err != nil {
		return fmt.Errorf("registerMinIOConnectors: parse MINIO_ENDPOINT %q: %w", rawEndpoint, err)
	}
	host := u.Host
	if host == "" {
		// Treat a bare "host:port" (no scheme) as the host directly.
		host = strings.TrimPrefix(rawEndpoint, "http://")
		host = strings.TrimPrefix(host, "https://")
	}
	useSSL := u.Scheme == "https"

	adapter, err := workerPkg.NewMinioClientAdapter(host, accessKey, secretKey, useSSL)
	if err != nil {
		return fmt.Errorf("registerMinIOConnectors: %w", err)
	}

	workerPkg.RegisterMinIOConnectors(reg, adapter)
	log.Printf("worker: MinIO connectors registered (endpoint=%s ssl=%v)", host, useSSL)
	return nil
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

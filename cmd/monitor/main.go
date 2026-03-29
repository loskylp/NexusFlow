// Command monitor is the entry point for the NexusFlow Monitor service.
// The Monitor runs as a single instance; it is not horizontally scaled.
//
// Startup sequence:
//  1. Load config (DATABASE_URL, REDIS_URL required)
//  2. Connect to PostgreSQL (run migrations)
//  3. Connect to Redis (verify with PING)
//  4. Build WorkerRepository, TaskRepository, HeartbeatStore, PendingScanner, Producer
//  5. Build SSE RedisBroker
//  6. Construct Monitor and call Run (blocks until SIGTERM/SIGINT)
//
// See: ADR-002, ADR-004, ADR-005, TASK-009 (Cycle 2)
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nxlabs/nexusflow/internal/config"
	"github.com/nxlabs/nexusflow/internal/db"
	"github.com/nxlabs/nexusflow/internal/queue"
	"github.com/nxlabs/nexusflow/internal/sse"
	monitorPkg "github.com/nxlabs/nexusflow/monitor"
	"github.com/redis/go-redis/v9"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("monitor: starting")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("monitor: configuration error: %v", err)
	}

	log.Printf("monitor: heartbeat-timeout=%v pending-scan-interval=%v env=%q",
		cfg.HeartbeatTimeout, cfg.PendingScanInterval, cfg.Env)

	// Connect to PostgreSQL. Migrations run automatically on successful connection.
	startupCtx, startupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer startupCancel()

	pool, err := db.New(startupCtx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("monitor: postgres: %v", err)
	}
	defer pool.Close()
	log.Printf("monitor: postgres connected")

	// Connect Redis.
	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Fatalf("monitor: invalid REDIS_URL %q: %v", cfg.RedisURL, err)
	}
	redisClient := redis.NewClient(redisOpts)
	defer func() { _ = redisClient.Close() }()

	if err := redisClient.Ping(startupCtx).Err(); err != nil {
		log.Fatalf("monitor: redis not reachable: %v", err)
	}
	log.Printf("monitor: redis connected at %s", cfg.RedisURL)

	// Build repositories.
	workerRepo := db.NewPgWorkerRepository(pool)
	taskRepo := db.NewPgTaskRepository(pool)

	// RedisQueue satisfies HeartbeatStore, PendingScanner, and Producer simultaneously.
	redisQueue := queue.NewRedisQueue(redisClient)

	// Build SSE broker for publishing worker-down and task-failed events.
	broker := sse.NewRedisBroker(redisClient)

	// Construct the monitor with all dependencies fully wired.
	m := monitorPkg.NewMonitor(
		cfg,
		workerRepo,
		taskRepo,
		redisQueue, // HeartbeatStore
		redisQueue, // PendingScanner
		redisQueue, // Producer
		broker,
	)

	// Run blocks until SIGTERM/SIGINT.
	runCtx, runCancel := context.WithCancel(context.Background())

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	errCh := make(chan error, 1)
	go func() {
		errCh <- m.Run(runCtx)
	}()

	select {
	case sig := <-quit:
		log.Printf("monitor: received signal %s — shutting down", sig)
		runCancel()
		if err := <-errCh; err != nil {
			log.Printf("monitor: Run returned: %v", err)
		}
	case err := <-errCh:
		runCancel()
		if err != nil {
			log.Fatalf("monitor: Run failed: %v", err)
		}
	}

	log.Printf("monitor: stopped cleanly")
}

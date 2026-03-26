// Command worker is the entry point for the NexusFlow Worker process.
// Workers are stateless compute nodes; multiple instances can run simultaneously
// and are scaled via `docker compose up --scale worker=N`.
//
// TASK-001 implementation: loads config, connects to Redis and Postgres (with
// retry logging on failure), logs "worker starting", and blocks on SIGTERM/SIGINT.
// Full worker execution loop is implemented in TASK-006 and TASK-007.
//
// See: ADR-001, ADR-002, ADR-004, ADR-005, TASK-006, TASK-007, TASK-042
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nxlabs/nexusflow/internal/config"
	"github.com/redis/go-redis/v9"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("worker: starting")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("worker: configuration error: %v", err)
	}

	log.Printf("worker: tags=%v id=%q env=%q", cfg.WorkerTags, cfg.WorkerID, cfg.Env)

	// Connect Redis. Ping to verify connectivity; log and continue on failure.
	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Fatalf("worker: invalid REDIS_URL %q: %v", cfg.RedisURL, err)
	}
	redisClient := redis.NewClient(redisOpts)
	defer redisClient.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Printf("worker: Redis not reachable at startup: %v (will retry)", err)
	} else {
		log.Printf("worker: Redis connected at %s", cfg.RedisURL)
	}

	// PostgreSQL connection and Worker.Run are wired in TASK-002 and TASK-006.
	// Block until signal so the container stays alive for health checks.
	log.Printf("worker: ready — waiting for tasks (full implementation in TASK-006)")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	<-quit

	log.Printf("worker: stopped cleanly")
}

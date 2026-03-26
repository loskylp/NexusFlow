// Command monitor is the entry point for the NexusFlow Monitor service.
// The Monitor runs as a single instance; it is not horizontally scaled.
//
// TASK-001 implementation: loads config, connects to Redis (with retry logging
// on failure), logs "monitor starting", and blocks on SIGTERM/SIGINT.
// Full monitor heartbeat and pending-entry scan loops are implemented in TASK-009 (Cycle 2).
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

	// Connect Redis. Ping to verify connectivity; log and continue on failure.
	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Fatalf("monitor: invalid REDIS_URL %q: %v", cfg.RedisURL, err)
	}
	redisClient := redis.NewClient(redisOpts)
	defer redisClient.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Printf("monitor: Redis not reachable at startup: %v (will retry)", err)
	} else {
		log.Printf("monitor: Redis connected at %s", cfg.RedisURL)
	}

	// Monitor.Run is wired in TASK-009 (Cycle 2).
	// Block until signal so the container stays alive.
	log.Printf("monitor: ready — monitoring disabled until TASK-009")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	<-quit

	log.Printf("monitor: stopped cleanly")
}

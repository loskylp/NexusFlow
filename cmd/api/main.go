// Command api is the entry point for the NexusFlow API server.
// Loads configuration from environment variables, wires dependencies, and starts
// the HTTP server on the configured APIPort.
//
// Service startup order:
//  1. Load config (config.Load)
//  2. Open PostgreSQL pool and run migrations (db.New — TASK-002)
//  3. Connect Redis client (go-redis)
//  4. Build API server (api.NewServer) with repository implementations wired from TASK-002
//  5. Start HTTP server with graceful shutdown on SIGTERM/SIGINT
//
// See: ADR-004, ADR-005, ADR-006, TASK-001, TASK-002, TASK-003
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nxlabs/nexusflow/api"
	"github.com/nxlabs/nexusflow/internal/config"
	"github.com/nxlabs/nexusflow/internal/db"
	"github.com/redis/go-redis/v9"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("api: configuration error: %v", err)
	}

	log.Printf("api: starting in %q environment on port %d", cfg.Env, cfg.APIPort)

	// Connect Redis. The client is lazy-connected; Ping verifies connectivity.
	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Fatalf("api: invalid REDIS_URL %q: %v", cfg.RedisURL, err)
	}
	redisClient := redis.NewClient(redisOpts)
	defer redisClient.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Printf("api: Redis not reachable at startup: %v (will retry on health checks)", err)
	} else {
		log.Printf("api: Redis connected at %s", cfg.RedisURL)
	}

	// Open PostgreSQL pool and run all pending schema migrations (TASK-002).
	startCtx, startCancel := context.WithTimeout(context.Background(), 30*time.Second)
	pool, err := db.New(startCtx, cfg.DatabaseURL)
	startCancel()
	if err != nil {
		log.Fatalf("api: cannot connect to PostgreSQL: %v", err)
	}
	defer pool.Close()
	log.Printf("api: PostgreSQL connected and migrations applied")

	// Repository implementations are wired in later tasks (TASK-003, TASK-005, etc.).
	// Pass nil for repositories not yet implemented; handler stubs return 500 until implemented.
	srv := api.NewServer(
		cfg,
		pool,
		redisClient,
		nil, // users — wired in TASK-003
		nil, // tasks — wired in TASK-005
		nil, // pipelines — wired in TASK-013
		nil, // workers — wired in TASK-006
		nil, // producer — wired in TASK-004
		nil, // sessions — wired in TASK-003
		nil, // broker — wired in TASK-015
	)

	addr := fmt.Sprintf(":%d", cfg.APIPort)
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      srv.Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown: wait for SIGTERM or SIGINT, then give in-flight requests 10 s.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		log.Printf("api: listening on %s", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("api: ListenAndServe error: %v", err)
		}
	}()

	<-quit
	log.Printf("api: shutdown signal received")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("api: graceful shutdown failed: %v", err)
	}
	log.Printf("api: stopped cleanly")
}

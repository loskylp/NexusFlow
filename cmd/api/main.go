// Command api is the entry point for the NexusFlow API server.
// Loads configuration from environment variables, wires dependencies, and starts
// the HTTP server on the configured APIPort.
//
// Service startup order:
//  1. Load config (config.Load)
//  2. Open PostgreSQL pool and run migrations (db.New — TASK-002)
//  3. Connect Redis client (go-redis)
//  4. Construct SessionStore and UserRepository (TASK-003)
//  5. Seed admin user if no users exist (TASK-003)
//  6. Construct TaskRepository, PipelineRepository, and RedisQueue Producer (TASK-005)
//  7. Build API server (api.NewServer) with all dependencies wired
//  8. Start HTTP server with graceful shutdown on SIGTERM/SIGINT
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

	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/api"
	"github.com/nxlabs/nexusflow/internal/auth"
	"github.com/nxlabs/nexusflow/internal/config"
	"github.com/nxlabs/nexusflow/internal/db"
	"github.com/nxlabs/nexusflow/internal/models"
	"github.com/nxlabs/nexusflow/internal/queue"
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

	// Wire TASK-003 dependencies: UserRepository and SessionStore.
	userRepo := db.NewPgUserRepository(pool)
	sessionStore := queue.NewRedisSessionStore(redisClient, 24*time.Hour)

	// Wire TASK-005 dependencies: TaskRepository, PipelineRepository, and queue Producer.
	taskRepo := db.NewPgTaskRepository(pool)
	pipelineRepo := db.NewPgPipelineRepository(pool)
	q := queue.NewRedisQueue(redisClient)

	// Seed the initial admin user if no users exist (TASK-003, AC-7).
	seedCtx, seedCancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := seedAdminIfEmpty(seedCtx, userRepo); err != nil {
		log.Printf("api: admin seed warning: %v", err)
	}
	seedCancel()

	srv := api.NewServer(
		cfg,
		pool,
		redisClient,
		userRepo,
		taskRepo,
		pipelineRepo,
		nil, // workers — wired in TASK-006
		q,
		sessionStore,
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

// seedAdminIfEmpty creates the default admin user when no users exist in the database.
// This satisfies TASK-003 AC-7: "admin user (admin/admin) is seeded if no users exist".
//
// Admin credentials: username=admin, password=admin (bcrypt-hashed at cost 12).
// Email/username domain: admin@nexusflow.local per TASK-003 spec.
//
// Args:
//
//	ctx:      Context for database operations.
//	userRepo: The UserRepository used to check and create users.
//
// Postconditions:
//   - If users exist: no action taken; returns nil.
//   - If no users exist: admin user is created and logged; returns nil on success.
//   - On failure: returns a non-fatal error (caller logs and continues startup).
func seedAdminIfEmpty(ctx context.Context, userRepo db.UserRepository) error {
	users, err := userRepo.List(ctx)
	if err != nil {
		return fmt.Errorf("seedAdmin: list users: %w", err)
	}
	if len(users) > 0 {
		return nil // Users exist; skip seeding.
	}

	hash, err := auth.HashPassword("admin")
	if err != nil {
		return fmt.Errorf("seedAdmin: hash password: %w", err)
	}

	admin := &models.User{
		ID:           uuid.New(),
		Username:     "admin",
		PasswordHash: hash,
		Role:         models.RoleAdmin,
		Active:       true,
		CreatedAt:    time.Now().UTC(),
	}

	if _, err := userRepo.Create(ctx, admin); err != nil {
		return fmt.Errorf("seedAdmin: create admin user: %w", err)
	}

	log.Printf("api: admin user seeded (username=admin) — change password immediately in production")
	return nil
}

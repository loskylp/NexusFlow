// Package config handles loading and validation of NexusFlow's runtime configuration.
// All configuration is sourced from environment variables (12-factor, ADR-005).
// A .env.example file at the project root documents every variable.
// See: ADR-005, TASK-001
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration for NexusFlow services.
// Load returns a populated Config or an error listing every missing required variable.
// Individual service binaries (cmd/api, cmd/worker, cmd/monitor) call Load on startup.
type Config struct {
	// DatabaseURL is the PostgreSQL connection string.
	// Required. Format: postgresql://user:password@host:port/dbname
	DatabaseURL string

	// RedisURL is the Redis connection string.
	// Required. Format: redis://host:port
	RedisURL string

	// SessionTTL is how long an authenticated session remains valid.
	// Default: 24h. See: ADR-006.
	SessionTTL time.Duration

	// HeartbeatInterval is how often a Worker emits a heartbeat to Redis.
	// Default: 5s. See: ADR-002.
	HeartbeatInterval time.Duration

	// HeartbeatTimeout is how long without a heartbeat before a Worker is declared down.
	// Default: 15s. See: ADR-002.
	HeartbeatTimeout time.Duration

	// PendingScanInterval is how often the Monitor scans for pending tasks from downed workers.
	// Default: 10s. See: ADR-002.
	PendingScanInterval time.Duration

	// LogHotRetention is how long log lines remain in Redis Streams.
	// Default: 72h. See: ADR-008.
	LogHotRetention time.Duration

	// LogColdRetention is how long log lines remain in PostgreSQL before partition pruning.
	// Default: 720h (30 days). See: ADR-008.
	LogColdRetention time.Duration

	// WorkerTags is the comma-separated list of capability tags for this Worker instance.
	// Populated only by cmd/worker. Example: "etl,report"
	// See: ADR-001, REQ-005, TASK-006.
	WorkerTags []string

	// WorkerID is a unique identifier for this Worker instance.
	// Generated on startup if not set (UUID or hostname-based). See: TASK-006.
	WorkerID string

	// APIPort is the TCP port the API server listens on.
	// Default: 8080. See: TASK-001.
	APIPort int

	// Env is the deployment environment ("development", "staging", "production").
	// Affects log verbosity and CORS policy.
	Env string
}

// Load reads configuration from environment variables, applies defaults,
// and returns an error for any missing required variable.
//
// Preconditions:
//   - Environment variables are set in the process environment or loaded from a .env file.
//
// Postconditions:
//   - On success: returned Config has all required fields populated and defaults applied.
//   - On failure: returned error lists every missing or invalid variable.
func Load() (*Config, error) {
	var missing []string

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		missing = append(missing, "REDIS_URL")
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	cfg := &Config{
		DatabaseURL:         databaseURL,
		RedisURL:            redisURL,
		SessionTTL:          parseDurationHours("SESSION_TTL_HOURS", 24),
		HeartbeatInterval:   parseDurationSeconds("HEARTBEAT_INTERVAL_SECONDS", 5),
		HeartbeatTimeout:    parseDurationSeconds("HEARTBEAT_TIMEOUT_SECONDS", 15),
		PendingScanInterval: parseDurationSeconds("PENDING_SCAN_INTERVAL_SECONDS", 10),
		LogHotRetention:     parseDurationHours("LOG_HOT_RETENTION_HOURS", 72),
		LogColdRetention:    parseDurationHours("LOG_COLD_RETENTION_HOURS", 720),
		WorkerTags:          parseWorkerTags(os.Getenv("WORKER_TAGS")),
		WorkerID:            os.Getenv("WORKER_ID"),
		APIPort:             parseInt("API_PORT", 8080),
		Env:                 envOrDefault("ENV", "development"),
	}

	return cfg, nil
}

// parseDurationHours parses an integer number of hours from the named environment
// variable and returns it as a time.Duration. Falls back to defaultHours if the
// variable is absent or cannot be parsed.
func parseDurationHours(envVar string, defaultHours int) time.Duration {
	raw := os.Getenv(envVar)
	if raw == "" {
		return time.Duration(defaultHours) * time.Hour
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return time.Duration(defaultHours) * time.Hour
	}
	return time.Duration(v) * time.Hour
}

// parseDurationSeconds parses an integer number of seconds from the named environment
// variable. Falls back to defaultSeconds if the variable is absent or unparseable.
func parseDurationSeconds(envVar string, defaultSeconds int) time.Duration {
	raw := os.Getenv(envVar)
	if raw == "" {
		return time.Duration(defaultSeconds) * time.Second
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return time.Duration(defaultSeconds) * time.Second
	}
	return time.Duration(v) * time.Second
}

// parseInt parses an integer from the named environment variable.
// Falls back to defaultVal if the variable is absent or unparseable.
func parseInt(envVar string, defaultVal int) int {
	raw := os.Getenv(envVar)
	if raw == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return defaultVal
	}
	return v
}

// envOrDefault returns the value of envVar, or fallback if the variable is empty.
func envOrDefault(envVar, fallback string) string {
	if v := os.Getenv(envVar); v != "" {
		return v
	}
	return fallback
}

// parseWorkerTags splits the raw WORKER_TAGS string by commas and trims whitespace
// from each tag. Returns nil if the input is empty.
func parseWorkerTags(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	tags := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}

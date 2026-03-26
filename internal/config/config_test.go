// Package config — unit tests for config.Load().
// Tests follow the red/green/refactor cycle: each test specifies a behaviour
// before the implementation exists.
// See: ADR-005, TASK-001
package config

import (
	"testing"
	"time"
)

// clearEnv blanks all NexusFlow environment variables so tests start from a known state.
// t.Setenv registers a cleanup that restores each variable's original value when the test ends.
// Blanking (rather than unsetting) is sufficient because config.Load treats empty strings
// identically to absent variables.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, v := range []string{
		"DATABASE_URL", "REDIS_URL", "SESSION_TTL_HOURS",
		"HEARTBEAT_INTERVAL_SECONDS", "HEARTBEAT_TIMEOUT_SECONDS",
		"PENDING_SCAN_INTERVAL_SECONDS", "LOG_HOT_RETENTION_HOURS",
		"LOG_COLD_RETENTION_HOURS", "WORKER_TAGS", "WORKER_ID",
		"API_PORT", "ENV",
	} {
		t.Setenv(v, "")
	}
}

// TestLoad_MissingRequired verifies that Load returns an error when required
// environment variables (DATABASE_URL and REDIS_URL) are absent.
func TestLoad_MissingRequired(t *testing.T) {
	clearEnv(t)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when required environment variables are missing, got nil")
	}
}

// TestLoad_RequiredPresent verifies that Load succeeds when DATABASE_URL and REDIS_URL
// are set, and that the returned Config reflects those values.
func TestLoad_RequiredPresent(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgresql://user:pass@localhost:5432/nexusflow")
	t.Setenv("REDIS_URL", "redis://localhost:6379")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DatabaseURL != "postgresql://user:pass@localhost:5432/nexusflow" {
		t.Errorf("DatabaseURL: got %q, want %q", cfg.DatabaseURL, "postgresql://user:pass@localhost:5432/nexusflow")
	}
	if cfg.RedisURL != "redis://localhost:6379" {
		t.Errorf("RedisURL: got %q, want %q", cfg.RedisURL, "redis://localhost:6379")
	}
}

// TestLoad_Defaults verifies that Load applies the documented defaults when optional
// environment variables are absent.
func TestLoad_Defaults(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgresql://user:pass@localhost:5432/nexusflow")
	t.Setenv("REDIS_URL", "redis://localhost:6379")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.SessionTTL != 24*time.Hour {
		t.Errorf("SessionTTL default: got %v, want 24h", cfg.SessionTTL)
	}
	if cfg.HeartbeatInterval != 5*time.Second {
		t.Errorf("HeartbeatInterval default: got %v, want 5s", cfg.HeartbeatInterval)
	}
	if cfg.HeartbeatTimeout != 15*time.Second {
		t.Errorf("HeartbeatTimeout default: got %v, want 15s", cfg.HeartbeatTimeout)
	}
	if cfg.PendingScanInterval != 10*time.Second {
		t.Errorf("PendingScanInterval default: got %v, want 10s", cfg.PendingScanInterval)
	}
	if cfg.LogHotRetention != 72*time.Hour {
		t.Errorf("LogHotRetention default: got %v, want 72h", cfg.LogHotRetention)
	}
	if cfg.LogColdRetention != 720*time.Hour {
		t.Errorf("LogColdRetention default: got %v, want 720h (30 days)", cfg.LogColdRetention)
	}
	if cfg.APIPort != 8080 {
		t.Errorf("APIPort default: got %d, want 8080", cfg.APIPort)
	}
	if cfg.Env != "development" {
		t.Errorf("Env default: got %q, want %q", cfg.Env, "development")
	}
}

// TestLoad_WorkerTags verifies that WORKER_TAGS is parsed into a string slice,
// splitting on commas, with surrounding whitespace trimmed.
func TestLoad_WorkerTags(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgresql://user:pass@localhost:5432/nexusflow")
	t.Setenv("REDIS_URL", "redis://localhost:6379")
	t.Setenv("WORKER_TAGS", "demo, etl , report")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.WorkerTags) != 3 {
		t.Fatalf("WorkerTags: got %d tags, want 3: %v", len(cfg.WorkerTags), cfg.WorkerTags)
	}
	want := []string{"demo", "etl", "report"}
	for i, tag := range want {
		if cfg.WorkerTags[i] != tag {
			t.Errorf("WorkerTags[%d]: got %q, want %q", i, cfg.WorkerTags[i], tag)
		}
	}
}

// TestLoad_APIPort verifies that a custom API_PORT overrides the default.
func TestLoad_APIPort(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgresql://user:pass@localhost:5432/nexusflow")
	t.Setenv("REDIS_URL", "redis://localhost:6379")
	t.Setenv("API_PORT", "9090")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.APIPort != 9090 {
		t.Errorf("APIPort: got %d, want 9090", cfg.APIPort)
	}
}

// TestLoad_MissingDatabaseURL verifies that only REDIS_URL present still fails.
func TestLoad_MissingDatabaseURL(t *testing.T) {
	clearEnv(t)
	t.Setenv("REDIS_URL", "redis://localhost:6379")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when DATABASE_URL is missing, got nil")
	}
}

// TestLoad_MissingRedisURL verifies that only DATABASE_URL present still fails.
func TestLoad_MissingRedisURL(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgresql://user:pass@localhost:5432/nexusflow")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when REDIS_URL is missing, got nil")
	}
}

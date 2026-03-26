// Package queue — unit tests for RedisSessionStore.
// These tests require a live Redis instance. Run with:
//   docker compose -f /path/to/docker-compose.yml up redis -d
// See: ADR-006, TASK-003
package queue

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/models"
	"github.com/redis/go-redis/v9"
)

// redisClientForTest returns a go-redis client pointed at the test Redis instance.
// Uses REDIS_URL env if set, otherwise defaults to localhost:6379.
func redisClientForTest(t *testing.T) *redis.Client {
	t.Helper()
	url := os.Getenv("REDIS_URL")
	if url == "" {
		url = "redis://localhost:6379/0"
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		t.Fatalf("redisClientForTest: invalid REDIS_URL: %v", err)
	}
	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available (%v) — skipping Redis integration tests", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestRedisSessionStore_CreateAndGet(t *testing.T) {
	client := redisClientForTest(t)
	store := NewRedisSessionStore(client, 24*time.Hour)

	ctx := context.Background()
	token := "test-create-get-" + uuid.New().String()
	sess := &models.Session{
		UserID:    uuid.New(),
		Role:      models.RoleUser,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}

	t.Cleanup(func() { _ = store.Delete(ctx, token) })

	if err := store.Create(ctx, token, sess); err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}

	got, err := store.Get(ctx, token)
	if err != nil {
		t.Fatalf("Get: unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("Get: expected session, got nil")
	}
	if got.UserID != sess.UserID {
		t.Errorf("UserID mismatch: got %v, want %v", got.UserID, sess.UserID)
	}
	if got.Role != sess.Role {
		t.Errorf("Role mismatch: got %v, want %v", got.Role, sess.Role)
	}
}

func TestRedisSessionStore_GetMissingTokenReturnsNil(t *testing.T) {
	client := redisClientForTest(t)
	store := NewRedisSessionStore(client, 24*time.Hour)

	ctx := context.Background()
	got, err := store.Get(ctx, "no-such-token-"+uuid.New().String())
	if err != nil {
		t.Fatalf("Get: expected nil error for missing token, got %v", err)
	}
	if got != nil {
		t.Errorf("Get: expected nil session for missing token, got %+v", got)
	}
}

func TestRedisSessionStore_DeleteInvalidatesSession(t *testing.T) {
	client := redisClientForTest(t)
	store := NewRedisSessionStore(client, 24*time.Hour)

	ctx := context.Background()
	token := "test-delete-" + uuid.New().String()
	sess := &models.Session{UserID: uuid.New(), Role: models.RoleUser, CreatedAt: time.Now()}

	_ = store.Create(ctx, token, sess)

	if err := store.Delete(ctx, token); err != nil {
		t.Fatalf("Delete: unexpected error: %v", err)
	}

	got, err := store.Get(ctx, token)
	if err != nil {
		t.Fatalf("Get after Delete: unexpected error: %v", err)
	}
	if got != nil {
		t.Error("Get after Delete: expected nil session, got non-nil")
	}
}

func TestRedisSessionStore_DeleteAllForUser(t *testing.T) {
	client := redisClientForTest(t)
	store := NewRedisSessionStore(client, 24*time.Hour)

	ctx := context.Background()
	userID := uuid.New()

	token1 := "test-all-1-" + uuid.New().String()
	token2 := "test-all-2-" + uuid.New().String()
	otherToken := "test-other-" + uuid.New().String()
	otherUser := uuid.New()

	sess1 := &models.Session{UserID: userID, Role: models.RoleUser, CreatedAt: time.Now()}
	sess2 := &models.Session{UserID: userID, Role: models.RoleUser, CreatedAt: time.Now()}
	otherSess := &models.Session{UserID: otherUser, Role: models.RoleAdmin, CreatedAt: time.Now()}

	_ = store.Create(ctx, token1, sess1)
	_ = store.Create(ctx, token2, sess2)
	_ = store.Create(ctx, otherToken, otherSess)
	t.Cleanup(func() {
		_ = store.Delete(ctx, token1)
		_ = store.Delete(ctx, token2)
		_ = store.Delete(ctx, otherToken)
	})

	if err := store.DeleteAllForUser(ctx, userID.String()); err != nil {
		t.Fatalf("DeleteAllForUser: unexpected error: %v", err)
	}

	// User's sessions should be gone.
	got1, _ := store.Get(ctx, token1)
	got2, _ := store.Get(ctx, token2)
	if got1 != nil || got2 != nil {
		t.Error("DeleteAllForUser: user sessions should be nil after deletion")
	}

	// Other user's session should remain.
	gotOther, _ := store.Get(ctx, otherToken)
	if gotOther == nil {
		t.Error("DeleteAllForUser: other user's session should not be deleted")
	}
}

func TestRedisSessionStore_TTLExpires(t *testing.T) {
	client := redisClientForTest(t)
	// Use a 1-second TTL to test expiry.
	store := NewRedisSessionStore(client, 1*time.Second)

	ctx := context.Background()
	token := "test-ttl-" + uuid.New().String()
	sess := &models.Session{UserID: uuid.New(), Role: models.RoleUser, CreatedAt: time.Now()}

	_ = store.Create(ctx, token, sess)

	// Wait for TTL to expire.
	time.Sleep(1500 * time.Millisecond)

	got, err := store.Get(ctx, token)
	if err != nil {
		t.Fatalf("Get after TTL: unexpected error: %v", err)
	}
	if got != nil {
		t.Error("Get after TTL: expected nil (expired), got non-nil")
	}
}

func TestNewRedisSessionStore_NilClientPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("NewRedisSessionStore(nil): expected panic, got none")
		}
	}()
	NewRedisSessionStore(nil, 24*time.Hour)
}

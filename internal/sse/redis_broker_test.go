// Package sse — unit tests for RedisBroker.
// Tests use in-memory stubs and httptest utilities; no live Redis or PostgreSQL is required.
// Covers: subscriber registry, pub/sub fan-out logic, writeSSEEvent formatting,
// backpressure drop behaviour, goroutine cleanup on disconnect, and access control.
// See: ADR-007, TASK-015
package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/models"
)

// --- httptest helpers ---

// sseRecorder wraps httptest.ResponseRecorder and implements http.Flusher.
// Used to capture SSE output in unit tests without a live HTTP server.
type sseRecorder struct {
	*httptest.ResponseRecorder
	flushed int
}

func (r *sseRecorder) Flush() { r.flushed++ }

func newSSERecorder() *sseRecorder {
	return &sseRecorder{ResponseRecorder: httptest.NewRecorder()}
}

// noFlushWriter is an http.ResponseWriter that does NOT implement http.Flusher.
// Used to verify that writeSSEEvent returns an error for incompatible writers.
type noFlushWriter struct {
	header http.Header
	body   strings.Builder
	code   int
}

func (w *noFlushWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}
func (w *noFlushWriter) Write(b []byte) (int, error) { return w.body.Write(b) }
func (w *noFlushWriter) WriteHeader(code int)        { w.code = code }

// --- writeSSEEvent ---

func TestWriteSSEEvent_FormatsIDEventData(t *testing.T) {
	w := newSSERecorder()
	evt := &models.SSEEvent{
		ID:      "42",
		Type:    "task:state-changed",
		Payload: map[string]string{"status": "running"},
	}

	if err := writeSSEEvent(w, evt); err != nil {
		t.Fatalf("writeSSEEvent returned error: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "id: 42\n") {
		t.Errorf("expected 'id: 42\\n' in body, got: %q", body)
	}
	if !strings.Contains(body, "event: task:state-changed\n") {
		t.Errorf("expected 'event: task:state-changed\\n' in body, got: %q", body)
	}
	if !strings.Contains(body, "data: ") {
		t.Errorf("expected 'data: ' in body, got: %q", body)
	}
	// Must end with double newline per SSE spec.
	if !strings.HasSuffix(body, "\n\n") {
		t.Errorf("expected body to end with '\\n\\n', got: %q", body)
	}
	if w.flushed == 0 {
		t.Error("expected Flush to be called at least once")
	}
}

func TestWriteSSEEvent_OmitsIDWhenEmpty(t *testing.T) {
	w := newSSERecorder()
	evt := &models.SSEEvent{
		Type:    "worker:down",
		Payload: "{}",
	}

	if err := writeSSEEvent(w, evt); err != nil {
		t.Fatalf("writeSSEEvent returned error: %v", err)
	}

	body := w.Body.String()
	if strings.Contains(body, "id:") {
		t.Errorf("expected no 'id:' line when ID is empty, got: %q", body)
	}
}

func TestWriteSSEEvent_PayloadIsValidJSON(t *testing.T) {
	w := newSSERecorder()
	type taskPayload struct {
		TaskID string `json:"taskId"`
		Status string `json:"status"`
	}
	evt := &models.SSEEvent{
		Type:    "task:completed",
		Payload: taskPayload{TaskID: "abc", Status: "completed"},
	}

	if err := writeSSEEvent(w, evt); err != nil {
		t.Fatalf("writeSSEEvent returned error: %v", err)
	}

	body := w.Body.String()
	var dataLine string
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "data: ") {
			dataLine = strings.TrimPrefix(line, "data: ")
			break
		}
	}
	if dataLine == "" {
		t.Fatalf("no 'data:' line found in: %q", body)
	}
	var got taskPayload
	if err := json.Unmarshal([]byte(dataLine), &got); err != nil {
		t.Errorf("data line is not valid JSON: %v — got: %q", err, dataLine)
	}
	if got.TaskID != "abc" || got.Status != "completed" {
		t.Errorf("unexpected payload: %+v", got)
	}
}

func TestWriteSSEEvent_ReturnsErrorForNonFlusher(t *testing.T) {
	w := &noFlushWriter{}
	evt := &models.SSEEvent{Type: "test", Payload: nil}

	err := writeSSEEvent(w, evt)
	if err == nil {
		t.Error("expected error for non-Flusher writer, got nil")
	}
}

// --- channel key helpers ---

func TestTaskChannelKey_UserRole(t *testing.T) {
	key := taskChannelKey("user-123", models.RoleUser)
	if key != "events:tasks:user-123" {
		t.Errorf("unexpected task channel key for user: %q", key)
	}
}

func TestTaskChannelKey_AdminRole(t *testing.T) {
	key := taskChannelKey("admin-1", models.RoleAdmin)
	if key != "events:tasks:all" {
		t.Errorf("unexpected task channel key for admin: %q", key)
	}
}

func TestLogChannelKey(t *testing.T) {
	key := logChannelKey("task-abc")
	if key != "events:logs:task-abc" {
		t.Errorf("unexpected log channel key: %q", key)
	}
}

func TestSinkChannelKey(t *testing.T) {
	key := sinkChannelKey("task-xyz")
	if key != "events:sink:task-xyz" {
		t.Errorf("unexpected sink channel key: %q", key)
	}
}

// --- subscriber registry ---

func TestRedisBroker_SubscribeReturnsBufferedChannel(t *testing.T) {
	b := NewRedisBroker(nil)

	ch := b.Subscribe("events:workers")
	if ch == nil {
		t.Fatal("Subscribe returned nil channel")
	}
	if cap(ch) == 0 {
		t.Error("Subscribe must return a buffered channel (capacity > 0)")
	}
}

func TestRedisBroker_UnsubscribeClosesChannel(t *testing.T) {
	b := NewRedisBroker(nil)

	ch := b.Subscribe("events:workers")
	b.Unsubscribe("events:workers", ch)

	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected closed channel after Unsubscribe; got a value instead")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("channel was not closed within 100 ms of Unsubscribe")
	}
}

func TestRedisBroker_UnsubscribeRemovesFromRegistry(t *testing.T) {
	b := NewRedisBroker(nil)

	ch := b.Subscribe("events:workers")
	b.Unsubscribe("events:workers", ch)

	b.mu.RLock()
	remaining := len(b.subscribers["events:workers"])
	b.mu.RUnlock()

	if remaining != 0 {
		t.Errorf("expected 0 subscribers after Unsubscribe, got %d", remaining)
	}
}

func TestRedisBroker_MultipleSubscribersTrackedSeparately(t *testing.T) {
	b := NewRedisBroker(nil)

	ch1 := b.Subscribe("events:workers")
	ch2 := b.Subscribe("events:workers")
	defer b.Unsubscribe("events:workers", ch1)
	defer b.Unsubscribe("events:workers", ch2)

	b.mu.RLock()
	count := len(b.subscribers["events:workers"])
	b.mu.RUnlock()

	if count != 2 {
		t.Errorf("expected 2 subscribers, got %d", count)
	}
}

// --- fan-out ---

func TestRedisBroker_FanOut_DeliveresToAllSubscribers(t *testing.T) {
	b := NewRedisBroker(nil)

	ch1 := b.Subscribe("events:workers")
	ch2 := b.Subscribe("events:workers")
	defer b.Unsubscribe("events:workers", ch1)
	defer b.Unsubscribe("events:workers", ch2)

	evt := &models.SSEEvent{Type: "worker:heartbeat", Payload: "{}"}
	b.fanOut("events:workers", evt)

	assertReceived(t, ch1, evt.Type, "ch1")
	assertReceived(t, ch2, evt.Type, "ch2")
}

func TestRedisBroker_FanOut_DoesNotBlockOnSlowConsumer(t *testing.T) {
	b := NewRedisBroker(nil)

	ch := b.Subscribe("events:workers")
	defer b.Unsubscribe("events:workers", ch)

	// Fill the buffer so the next send would block on a blocking channel send.
	filler := &models.SSEEvent{Type: "filler", Payload: nil}
	for i := 0; i < cap(ch); i++ {
		ch <- filler
	}

	done := make(chan struct{})
	go func() {
		b.fanOut("events:workers", &models.SSEEvent{Type: "dropped", Payload: nil})
		close(done)
	}()

	select {
	case <-done:
		// Good — fanOut returned without blocking.
	case <-time.After(200 * time.Millisecond):
		t.Error("fanOut blocked on a full-buffer subscriber instead of dropping the event")
	}
}

// --- goroutine / connection lifecycle ---

func TestRedisBroker_ServeWorkerEvents_ReturnsOnContextCancel(t *testing.T) {
	b := NewRedisBroker(nil)

	ctx, cancel := context.WithCancel(context.Background())
	session := &models.Session{
		UserID: uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"),
		Role:   models.RoleUser,
	}

	r := httptest.NewRequest(http.MethodGet, "/events/workers", nil).WithContext(ctx)
	w := newSSERecorder()

	done := make(chan struct{})
	go func() {
		b.ServeWorkerEvents(w, r, session)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Error("ServeWorkerEvents did not return within 500 ms after context cancellation")
	}
}

func TestRedisBroker_ServeWorkerEvents_CleansUpSubscriberOnDisconnect(t *testing.T) {
	b := NewRedisBroker(nil)

	ctx, cancel := context.WithCancel(context.Background())
	session := &models.Session{
		UserID: uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"),
		Role:   models.RoleUser,
	}

	r := httptest.NewRequest(http.MethodGet, "/events/workers", nil).WithContext(ctx)
	w := newSSERecorder()

	done := make(chan struct{})
	go func() {
		b.ServeWorkerEvents(w, r, session)
		close(done)
	}()

	cancel()
	<-done

	b.mu.RLock()
	count := len(b.subscribers["events:workers"])
	b.mu.RUnlock()

	if count != 0 {
		t.Errorf("expected 0 subscribers after disconnect, got %d", count)
	}
}

func TestRedisBroker_ServeTaskEvents_ReturnsOnContextCancel(t *testing.T) {
	b := NewRedisBroker(nil)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	session := &models.Session{
		UserID: uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"),
		Role:   models.RoleUser,
	}

	r := httptest.NewRequest(http.MethodGet, "/events/tasks", nil).WithContext(ctx)
	w := newSSERecorder()

	done := make(chan struct{})
	go func() {
		b.ServeTaskEvents(w, r, session)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Error("ServeTaskEvents did not return within 500 ms after context timeout")
	}
}

// --- SSE headers ---

func TestRedisBroker_ServeWorkerEvents_SetsSSEContentType(t *testing.T) {
	b := NewRedisBroker(nil)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	session := &models.Session{
		UserID: uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"),
		Role:   models.RoleUser,
	}
	r := httptest.NewRequest(http.MethodGet, "/events/workers", nil).WithContext(ctx)
	w := newSSERecorder()

	b.ServeWorkerEvents(w, r, session)

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Errorf("expected Content-Type text/event-stream, got %q", ct)
	}
}

func TestRedisBroker_ServeTaskEvents_SetsSSEContentType(t *testing.T) {
	b := NewRedisBroker(nil)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	session := &models.Session{
		UserID: uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"),
		Role:   models.RoleUser,
	}
	r := httptest.NewRequest(http.MethodGet, "/events/tasks", nil).WithContext(ctx)
	w := newSSERecorder()

	b.ServeTaskEvents(w, r, session)

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Errorf("expected Content-Type text/event-stream, got %q", ct)
	}
}

// --- access control ---

func TestRedisBroker_ServeLogEvents_Returns403WhenNotOwnerOrAdmin(t *testing.T) {
	b := NewRedisBroker(nil)

	ownerID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	callerID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	taskID := uuid.New()

	b.tasks = &stubTaskRepoForSSE{
		task: &models.Task{
			ID:     taskID,
			UserID: ownerID,
		},
	}

	session := &models.Session{
		UserID: callerID,
		Role:   models.RoleUser,
	}

	w := newSSERecorder()
	r := httptest.NewRequest(http.MethodGet, "/events/tasks/"+taskID.String()+"/logs", nil)

	b.ServeLogEvents(w, r, session, taskID.String())

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden, got %d", w.Code)
	}
}

func TestRedisBroker_ServeSinkEvents_Returns403WhenNotOwnerOrAdmin(t *testing.T) {
	b := NewRedisBroker(nil)

	ownerID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	callerID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	taskID := uuid.New()

	b.tasks = &stubTaskRepoForSSE{
		task: &models.Task{
			ID:     taskID,
			UserID: ownerID,
		},
	}

	session := &models.Session{
		UserID: callerID,
		Role:   models.RoleUser,
	}

	w := newSSERecorder()
	r := httptest.NewRequest(http.MethodGet, "/events/sink/"+taskID.String(), nil)

	b.ServeSinkEvents(w, r, session, taskID.String())

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden, got %d", w.Code)
	}
}

func TestRedisBroker_ServeLogEvents_AdminBypasses403Check(t *testing.T) {
	b := NewRedisBroker(nil)

	ownerID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	adminID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	taskID := uuid.New()

	b.tasks = &stubTaskRepoForSSE{
		task: &models.Task{
			ID:     taskID,
			UserID: ownerID,
		},
	}

	session := &models.Session{
		UserID: adminID,
		Role:   models.RoleAdmin,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	r := httptest.NewRequest(http.MethodGet, "/events/tasks/"+taskID.String()+"/logs", nil).WithContext(ctx)
	w := newSSERecorder()

	b.ServeLogEvents(w, r, session, taskID.String())

	if w.Code == http.StatusForbidden {
		t.Error("admin must not receive 403 on ServeLogEvents")
	}
}

func TestRedisBroker_ServeLogEvents_Returns403WhenTaskNotFound(t *testing.T) {
	b := NewRedisBroker(nil)

	callerID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	// No task in the stub — GetByID returns nil, nil.
	b.tasks = &stubTaskRepoForSSE{task: nil}

	session := &models.Session{
		UserID: callerID,
		Role:   models.RoleUser,
	}

	w := newSSERecorder()
	r := httptest.NewRequest(http.MethodGet, "/events/tasks/nonexistent/logs", nil)

	b.ServeLogEvents(w, r, session, "nonexistent-task-id")

	// A non-existent task is not accessible by this user.
	if w.Code != http.StatusForbidden && w.Code != http.StatusNotFound {
		t.Errorf("expected 403 or 404 for non-existent task, got %d", w.Code)
	}
}

// --- publish with nil client ---

func TestRedisBroker_PublishTaskEvent_ErrorsWithNilClient(t *testing.T) {
	b := NewRedisBroker(nil)
	task := &models.Task{
		UserID: uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"),
		Status: models.TaskStatusRunning,
	}

	err := b.PublishTaskEvent(context.Background(), task, "test reason")
	if err == nil {
		t.Error("expected error when Redis client is nil, got nil")
	}
}

func TestRedisBroker_PublishWorkerEvent_ErrorsWithNilClient(t *testing.T) {
	b := NewRedisBroker(nil)
	worker := &models.Worker{ID: "w-1", Status: models.WorkerStatusOnline}

	err := b.PublishWorkerEvent(context.Background(), worker)
	if err == nil {
		t.Error("expected error when Redis client is nil, got nil")
	}
}

func TestRedisBroker_PublishLogLine_ErrorsWithNilClient(t *testing.T) {
	b := NewRedisBroker(nil)
	logEntry := &models.TaskLog{Line: "hello", Level: "info"}

	err := b.PublishLogLine(context.Background(), logEntry)
	if err == nil {
		t.Error("expected error when Redis client is nil, got nil")
	}
}

func TestRedisBroker_PublishSinkSnapshot_ErrorsWithNilClient(t *testing.T) {
	b := NewRedisBroker(nil)
	snap := &models.SinkSnapshot{Phase: "before"}

	err := b.PublishSinkSnapshot(context.Background(), snap)
	if err == nil {
		t.Error("expected error when Redis client is nil, got nil")
	}
}

// --- concurrent safety ---

func TestRedisBroker_ConcurrentSubscribeUnsubscribeIsSafe(t *testing.T) {
	b := NewRedisBroker(nil)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			ch := b.Subscribe(fmt.Sprintf("events:tasks:user-%d", n))
			time.Sleep(time.Millisecond)
			b.Unsubscribe(fmt.Sprintf("events:tasks:user-%d", n), ch)
		}(i)
	}
	wg.Wait()

	b.mu.RLock()
	defer b.mu.RUnlock()
	for key, subs := range b.subscribers {
		if len(subs) != 0 {
			t.Errorf("channel %q still has %d subscribers after all goroutines exited", key, len(subs))
		}
	}
}

// --- stubs ---

// stubTaskRepoForSSE provides a minimal TaskRepository for access control tests.
// Only GetByID is implemented; all other methods return unimplemented panics.
type stubTaskRepoForSSE struct {
	task    *models.Task
	getErr  error
}

func (s *stubTaskRepoForSSE) GetByID(_ context.Context, _ uuid.UUID) (*models.Task, error) {
	return s.task, s.getErr
}

// --- helpers ---

func assertReceived(t *testing.T, ch <-chan *models.SSEEvent, wantType string, label string) {
	t.Helper()
	select {
	case got := <-ch:
		if got.Type != wantType {
			t.Errorf("%s: expected event type %q, got %q", label, wantType, got.Type)
		}
	case <-time.After(200 * time.Millisecond):
		t.Errorf("%s: no event received within 200 ms", label)
	}
}

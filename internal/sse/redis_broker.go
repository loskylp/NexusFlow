// Package sse — Redis-backed implementation of Broker.
// Subscribes to Redis Pub/Sub; fans events out to connected SSE http.ResponseWriters.
// See: ADR-007, TASK-015
package sse

import (
	"context"
	"net/http"
	"sync"

	"github.com/nxlabs/nexusflow/internal/models"
	"github.com/redis/go-redis/v9"
)

// RedisBroker implements Broker using Redis Pub/Sub for event distribution
// and an in-process subscriber registry for fan-out to SSE connections.
// See: ADR-007, TASK-015
type RedisBroker struct {
	client      *redis.Client                                  //lint:ignore U1000 scaffold stub — wired in TASK-015
	mu          sync.RWMutex                                   //lint:ignore U1000 scaffold stub — wired in TASK-015
	// subscribers maps a channel key to a set of send-only event channels,
	// one per connected SSE client.
	subscribers map[string]map[chan *models.SSEEvent]struct{} //lint:ignore U1000 scaffold stub — wired in TASK-015
}

// NewRedisBroker constructs a RedisBroker.
//
// Args:
//   client: A connected go-redis client. Must not be nil.
//
// Preconditions:
//   - client is connected and can SUBSCRIBE.
func NewRedisBroker(client *redis.Client) *RedisBroker {
	// TODO: Implement in TASK-015
	panic("not implemented")
}

// Start implements Broker.Start.
// Subscribes to all SSE-relevant Redis Pub/Sub channels and routes inbound messages
// to connected client goroutines. Blocks until ctx is cancelled.
// See: ADR-007, TASK-015
func (b *RedisBroker) Start(ctx context.Context) error {
	// TODO: Implement in TASK-015
	panic("not implemented")
}

// ServeTaskEvents implements Broker.ServeTaskEvents.
// See: ADR-007, TASK-015
func (b *RedisBroker) ServeTaskEvents(w http.ResponseWriter, r *http.Request, session *models.Session) {
	// TODO: Implement in TASK-015
	panic("not implemented")
}

// ServeWorkerEvents implements Broker.ServeWorkerEvents.
// See: ADR-007, TASK-015
func (b *RedisBroker) ServeWorkerEvents(w http.ResponseWriter, r *http.Request, session *models.Session) {
	// TODO: Implement in TASK-015
	panic("not implemented")
}

// ServeLogEvents implements Broker.ServeLogEvents.
// Complexity note: reconnection replay requires a PostgreSQL query for logs after
// Last-Event-ID, then live streaming from Redis Pub/Sub. Goroutine management
// and backpressure handling make this the most complex SSE endpoint.
// See: ADR-007, ADR-008, TASK-015
func (b *RedisBroker) ServeLogEvents(w http.ResponseWriter, r *http.Request, session *models.Session, taskID string) {
	// TODO: Implement in TASK-015
	panic("not implemented")
}

// ServeSinkEvents implements Broker.ServeSinkEvents.
// See: ADR-007, ADR-009, TASK-015
func (b *RedisBroker) ServeSinkEvents(w http.ResponseWriter, r *http.Request, session *models.Session, taskID string) {
	// TODO: Implement in TASK-015
	panic("not implemented")
}

// PublishTaskEvent implements Broker.PublishTaskEvent.
// See: ADR-007, TASK-015
func (b *RedisBroker) PublishTaskEvent(ctx context.Context, task *models.Task, reason string) error {
	// TODO: Implement in TASK-015
	panic("not implemented")
}

// PublishWorkerEvent implements Broker.PublishWorkerEvent.
// See: ADR-007, TASK-015
func (b *RedisBroker) PublishWorkerEvent(ctx context.Context, worker *models.Worker) error {
	// TODO: Implement in TASK-015
	panic("not implemented")
}

// PublishLogLine implements Broker.PublishLogLine.
// See: ADR-007, ADR-008, TASK-015
func (b *RedisBroker) PublishLogLine(ctx context.Context, log *models.TaskLog) error {
	// TODO: Implement in TASK-015
	panic("not implemented")
}

// PublishSinkSnapshot implements Broker.PublishSinkSnapshot.
// See: ADR-007, ADR-009, TASK-015
func (b *RedisBroker) PublishSinkSnapshot(ctx context.Context, snapshot *models.SinkSnapshot) error {
	// TODO: Implement in TASK-015
	panic("not implemented")
}

// writeSSEEvent writes a single SSE event to w and flushes the connection.
// Format:
//
//	id: {id}\n
//	event: {type}\n
//	data: {json}\n\n
//
// The id field is omitted if event.ID is empty.
// Returns an error if the write or flush fails (client disconnected).
//lint:ignore U1000 scaffold stub — wired in TASK-015
func writeSSEEvent(w http.ResponseWriter, event *models.SSEEvent) error {
	// TODO: Implement in TASK-015
	panic("not implemented")
}

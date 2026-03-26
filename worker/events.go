// Package worker — task event publishing interface for the Worker.
// The Worker needs only a subset of sse.Broker: the methods that publish events
// to Redis Pub/Sub. Using a narrow interface here (Interface Segregation, SOLID)
// avoids importing net/http in the worker and makes the Worker easier to test.
// See: ADR-007, TASK-007, TASK-015
package worker

import (
	"context"

	"github.com/nxlabs/nexusflow/internal/models"
)

// TaskEventBroker is the subset of sse.Broker that the Worker calls during
// task execution. It publishes events after each state transition.
//
// The full sse.Broker interface (which includes HTTP SSE server methods) satisfies
// this interface, so no adapter is required in production — only in tests where
// net/http types are undesirable.
//
// See: ADR-007, TASK-007
type TaskEventBroker interface {
	// PublishTaskEvent publishes a task state-change event to the Redis Pub/Sub channels
	// consumed by ServeTaskEvents handlers.
	//
	// Args:
	//   ctx:    Request context.
	//   task:   The task after its state transition.
	//   reason: Human-readable reason for the transition.
	//
	// Postconditions:
	//   - On success: event is published to events:tasks:{userId} and events:tasks:all.
	//   - On failure: error logged; SSE clients may miss this event (fire-and-forget, ADR-007).
	PublishTaskEvent(ctx context.Context, task *models.Task, reason string) error
}


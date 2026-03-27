// Package worker — internal test helpers.
// This file is in package worker (not package worker_test) so it can access
// unexported methods. It is compiled only during testing (the _test.go suffix
// ensures the Go toolchain excludes it from production builds).
//
// The exported symbols here are available to package worker_test.
// See: https://pkg.go.dev/cmd/go (build constraints, _test.go files)
package worker

import (
	"context"

	"github.com/nxlabs/nexusflow/internal/queue"
)

// ExecuteTaskForTest exposes the unexported executeTask method to the external
// test package (package worker_test). This is the conventional Go "export_test.go"
// pattern: a _test.go file in the production package that re-exports internals
// only for test use.
//
// This function is not compiled into production builds.
func (w *Worker) ExecuteTaskForTest(ctx context.Context, msg *queue.TaskMessage) {
	w.executeTask(ctx, msg)
}

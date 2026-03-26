// Package api — Health check handler.
// GET /api/health is unauthenticated and checks Redis and PostgreSQL connectivity.
// Monitored externally by Uptime Kuma via the kuma Docker label (ADR-005).
// See: ADR-005, TASK-001
package api

import (
	"encoding/json"
	"net/http"
)

// HealthHandler handles GET /api/health.
type HealthHandler struct {
	server *Server
}

// healthResponse is the JSON body returned by the health endpoint.
type healthResponse struct {
	Status   string `json:"status"`
	Redis    string `json:"redis"`
	Postgres string `json:"postgres"`
}

// Health handles GET /api/health.
// Checks Redis connectivity (PING) and PostgreSQL connectivity (SELECT 1).
// Returns 200 if all checks pass, 503 if any dependency is unreachable.
//
// Response body:
//
//	200: { "status": "ok",  "redis": "ok", "postgres": "ok" }
//	503: { "status": "degraded", "redis": "ok"|"error", "postgres": "ok"|"error" }
//
// Postconditions:
//   - Always returns a JSON body; never returns an empty response.
//   - Does not require authentication.
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	resp := healthResponse{
		Status:   "ok",
		Redis:    "ok",
		Postgres: "ok",
	}
	statusCode := http.StatusOK

	// Check Redis connectivity via PING.
	if h.server.redis == nil {
		resp.Redis = "error"
		resp.Status = "degraded"
		statusCode = http.StatusServiceUnavailable
	} else if err := h.server.redis.Ping(ctx).Err(); err != nil {
		resp.Redis = "error"
		resp.Status = "degraded"
		statusCode = http.StatusServiceUnavailable
	}

	// Check PostgreSQL connectivity via SELECT 1.
	// pool is nil until TASK-002 wires the database layer; report "error" until then.
	if h.server.pool == nil {
		resp.Postgres = "error"
		resp.Status = "degraded"
		statusCode = http.StatusServiceUnavailable
	} else if _, err := h.server.pool.Exec(ctx, "SELECT 1"); err != nil {
		resp.Postgres = "error"
		resp.Status = "degraded"
		statusCode = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	// A write error here indicates the client disconnected. Nothing to recover.
	_ = json.NewEncoder(w).Encode(resp)
}

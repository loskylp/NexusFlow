// Package db — unit tests for DSN conversion and migration helpers.
// Tests that require a live PostgreSQL database are integration tests
// and live in tests/integration/ (Verifier domain).
// See: ADR-008, TASK-002
package db

import (
	"testing"
)

// TestToPgx5DSN verifies that toPgx5DSN converts standard PostgreSQL DSN
// schemes to the pgx5:// scheme required by the golang-migrate pgx/v5 driver.
func TestToPgx5DSN(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "postgresql scheme is converted",
			input: "postgresql://user:pass@localhost:5432/mydb",
			want:  "pgx5://user:pass@localhost:5432/mydb",
		},
		{
			name:  "postgres scheme is converted",
			input: "postgres://user:pass@localhost/mydb",
			want:  "pgx5://user:pass@localhost/mydb",
		},
		{
			name:  "pgx5 scheme is returned unchanged",
			input: "pgx5://user:pass@localhost:5432/mydb",
			want:  "pgx5://user:pass@localhost:5432/mydb",
		},
		{
			name:  "DSN with query parameters is preserved",
			input: "postgresql://user:pass@host:5432/db?sslmode=disable",
			want:  "pgx5://user:pass@host:5432/db?sslmode=disable",
		},
		{
			name:  "empty string is returned unchanged",
			input: "",
			want:  "",
		},
		{
			name:  "unrecognised scheme is returned unchanged",
			input: "mysql://user:pass@host/db",
			want:  "mysql://user:pass@host/db",
		},
	}

	for _, tc := range cases {
		tc := tc // capture for t.Parallel
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := toPgx5DSN(tc.input)
			if got != tc.want {
				t.Errorf("toPgx5DSN(%q) = %q; want %q", tc.input, got, tc.want)
			}
		})
	}
}

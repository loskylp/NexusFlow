// Package retention — unit tests for log retention functions.
//
// Tests that require a live PostgreSQL database or Redis instance are integration
// tests and live in tests/integration/ (Verifier domain).
// These unit tests exercise the pure-logic helpers extracted from the main functions.
//
// See: ADR-008, TASK-028
package retention

import (
	"fmt"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// partitionNameFromBounds tests
// ---------------------------------------------------------------------------

// TestPartitionNameFromBounds_CorrectFormat verifies that partitionNameFromBounds
// produces names matching the task_logs_YYYY_WW convention defined in ADR-008.
func TestPartitionNameFromBounds_CorrectFormat(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		year int
		week int
		want string
	}{
		{
			name: "single-digit week is zero-padded",
			year: 2026,
			week: 4,
			want: "task_logs_2026_04",
		},
		{
			name: "double-digit week is not padded",
			year: 2026,
			week: 14,
			want: "task_logs_2026_14",
		},
		{
			name: "week 52 at year boundary",
			year: 2025,
			week: 52,
			want: "task_logs_2025_52",
		},
		{
			name: "week 1 of new year",
			year: 2027,
			week: 1,
			want: "task_logs_2027_01",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := partitionNameFromBounds(tc.year, tc.week)
			if got != tc.want {
				t.Errorf("partitionNameFromBounds(%d, %d) = %q; want %q", tc.year, tc.week, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// weekBounds tests
// ---------------------------------------------------------------------------

// TestWeekBounds_MondayIsStart verifies that weekBounds returns a Monday start
// and Monday end (exclusive) for the given time, as required by ISO 8601 week numbering.
func TestWeekBounds_MondayIsStart(t *testing.T) {
	t.Parallel()

	// 2026-04-07 is a Tuesday (ISO week 15, 2026).
	tuesday := time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC)
	start, end := weekBounds(tuesday)

	// Monday 2026-04-06 00:00:00 UTC
	wantStart := time.Date(2026, 4, 6, 0, 0, 0, 0, time.UTC)
	// Monday 2026-04-13 00:00:00 UTC (next week start, exclusive upper bound)
	wantEnd := time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC)

	if !start.Equal(wantStart) {
		t.Errorf("weekBounds start: got %v; want %v", start, wantStart)
	}
	if !end.Equal(wantEnd) {
		t.Errorf("weekBounds end: got %v; want %v", end, wantEnd)
	}
}

// TestWeekBounds_MondayInput verifies that Monday 00:00:00 produces itself as start.
func TestWeekBounds_MondayInput(t *testing.T) {
	t.Parallel()

	// 2026-04-06 is a Monday.
	monday := time.Date(2026, 4, 6, 0, 0, 0, 0, time.UTC)
	start, end := weekBounds(monday)

	wantStart := time.Date(2026, 4, 6, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC)

	if !start.Equal(wantStart) {
		t.Errorf("weekBounds start for Monday: got %v; want %v", start, wantStart)
	}
	if !end.Equal(wantEnd) {
		t.Errorf("weekBounds end for Monday: got %v; want %v", end, wantEnd)
	}
}

// TestWeekBounds_SundayInput verifies that Sunday is treated as the last day of its week,
// not as the start of a new week (Go uses Sunday=0; ISO 8601 uses Monday=1).
func TestWeekBounds_SundayInput(t *testing.T) {
	t.Parallel()

	// 2026-04-12 is a Sunday (still in week 15, ISO week 2026-W15).
	sunday := time.Date(2026, 4, 12, 23, 59, 59, 0, time.UTC)
	start, end := weekBounds(sunday)

	// The week containing that Sunday starts Monday 2026-04-06.
	wantStart := time.Date(2026, 4, 6, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC)

	if !start.Equal(wantStart) {
		t.Errorf("weekBounds start for Sunday: got %v; want %v", start, wantStart)
	}
	if !end.Equal(wantEnd) {
		t.Errorf("weekBounds end for Sunday: got %v; want %v", end, wantEnd)
	}
}

// ---------------------------------------------------------------------------
// isPartitionOlderThan30Days tests
// ---------------------------------------------------------------------------

// TestIsPartitionOlderThan30Days_OldPartitionIsDropped verifies that a partition
// whose end bound is more than 30 days in the past is classified as old.
func TestIsPartitionOlderThan30Days_OldPartitionIsDropped(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC)

	// A partition ending 31 days ago should be dropped.
	oldEnd := now.Add(-31 * 24 * time.Hour)
	if !isPartitionOlderThan30Days(oldEnd, now) {
		t.Error("expected partition with end 31 days ago to be old; got not old")
	}
}

// TestIsPartitionOlderThan30Days_RecentPartitionIsKept verifies that a partition
// whose end bound is fewer than 30 days in the past is not dropped.
func TestIsPartitionOlderThan30Days_RecentPartitionIsKept(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC)

	// A partition ending 29 days ago should be kept.
	recentEnd := now.Add(-29 * 24 * time.Hour)
	if isPartitionOlderThan30Days(recentEnd, now) {
		t.Error("expected partition with end 29 days ago to be kept; got old")
	}
}

// TestIsPartitionOlderThan30Days_ExactBoundaryIsKept verifies that exactly-30-day-old
// partitions are kept (boundary is exclusive).
func TestIsPartitionOlderThan30Days_ExactBoundaryIsKept(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC)

	// Exactly 30 days ago (on the boundary).
	boundaryEnd := now.Add(-30 * 24 * time.Hour)
	if isPartitionOlderThan30Days(boundaryEnd, now) {
		t.Error("expected exactly-30-day-old partition to be kept; got old")
	}
}

// ---------------------------------------------------------------------------
// parsePartitionDate tests
// ---------------------------------------------------------------------------

// TestParsePartitionDate_ValidName verifies that a well-formed partition name
// is parsed into the correct year and ISO week number.
func TestParsePartitionDate_ValidName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		input    string
		wantYear int
		wantWeek int
		wantErr  bool
	}{
		{
			name:     "standard partition name",
			input:    "task_logs_2026_14",
			wantYear: 2026,
			wantWeek: 14,
		},
		{
			name:     "zero-padded week",
			input:    "task_logs_2026_04",
			wantYear: 2026,
			wantWeek: 4,
		},
		{
			name:    "wrong prefix is rejected",
			input:   "other_table_2026_14",
			wantErr: true,
		},
		{
			name:    "too few parts is rejected",
			input:   "task_logs_2026",
			wantErr: true,
		},
		{
			name:    "non-numeric year is rejected",
			input:   "task_logs_XXXX_14",
			wantErr: true,
		},
		{
			name:    "non-numeric week is rejected",
			input:   "task_logs_2026_XX",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			year, week, err := parsePartitionDate(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("parsePartitionDate(%q): expected error; got year=%d week=%d", tc.input, year, week)
				}
				return
			}
			if err != nil {
				t.Fatalf("parsePartitionDate(%q): unexpected error: %v", tc.input, err)
			}
			if year != tc.wantYear {
				t.Errorf("parsePartitionDate(%q) year = %d; want %d", tc.input, year, tc.wantYear)
			}
			if week != tc.wantWeek {
				t.Errorf("parsePartitionDate(%q) week = %d; want %d", tc.input, week, tc.wantWeek)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// hotLogCutoffID tests
// ---------------------------------------------------------------------------

// TestHotLogCutoffID_FormatIsMinID verifies that hotLogCutoffID produces a Redis
// Stream MINID-compatible string: "<unix-ms>-0".
func TestHotLogCutoffID_FormatIsMinID(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC)
	id := hotLogCutoffID(now)

	cutoffMs := now.Add(-hotLogMaxAgeHours * time.Hour).UnixMilli()
	want := fmt.Sprintf("%d-0", cutoffMs)

	if id != want {
		t.Errorf("hotLogCutoffID: got %q; want %q", id, want)
	}
}

// TestHotLogCutoffID_72HourWindow verifies that the cutoff is exactly 72 hours before now.
func TestHotLogCutoffID_72HourWindow(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	id := hotLogCutoffID(now)

	cutoffMs := now.Add(-72 * time.Hour).UnixMilli()
	want := fmt.Sprintf("%d-0", cutoffMs)

	if id != want {
		t.Errorf("hotLogCutoffID: got %q; want %q", id, want)
	}
}

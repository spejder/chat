package quiet_test

import (
	"testing"
	"time"

	"github.com/spejder/chat/internal/quiet"
)

// TestTheNightHasEdges checks the minutes around the start and the end of
// the night, and a moment that arrives in UTC.
func TestTheNightHasEdges(t *testing.T) {
	t.Parallel()

	at := func(hour, minute int) time.Time {
		return time.Date(2026, time.October, 6, hour, minute, 0, 0, quiet.Location())
	}

	for _, test := range []struct {
		at    time.Time
		night bool
	}{
		{at(21, 59), false},
		{at(22, 0), true},
		{at(3, 0), true},
		{at(6, 59), true},
		{at(7, 0), false},
		{at(12, 0), false},
		// 20:30 UTC is 22:30 in Copenhagen in the summer time.
		{time.Date(2026, time.October, 6, 20, 30, 0, 0, time.UTC), true},
		// 20:30 UTC is 21:30 in Copenhagen in the winter time.
		{time.Date(2026, time.December, 6, 20, 30, 0, 0, time.UTC), false},
	} {
		if got := quiet.Night(test.at); got != test.night {
			t.Errorf("Night(%v) = %v, want %v", test.at, got, test.night)
		}
	}
}

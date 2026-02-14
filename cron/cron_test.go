package cron

import (
	"testing"
	"time"
)

func TestCronStopTwiceShouldNotPanic(t *testing.T) {
	c := NewCron()

	c.Stop()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("stopping cron twice should not panic: %v", r)
		}
	}()

	c.Stop()
}

func TestParseInvalidSpecShouldReturnError(t *testing.T) {
	if _, err := Parse("not-a-valid-cron-spec"); err == nil {
		t.Fatalf("expected parse error for invalid cron spec")
	}
}

func TestParseEveryNMinutes(t *testing.T) {
	tests := []struct {
		spec     string
		expected time.Duration
	}{
		{spec: "every 1 minutes", expected: 1 * time.Minute},
		{spec: "every 5 minutes", expected: 5 * time.Minute},
		{spec: "every 30 minutes", expected: 30 * time.Minute},
	}

	base := time.Date(2026, 2, 13, 10, 0, 0, 0, time.UTC)
	for _, tc := range tests {
		t.Run(tc.spec, func(t *testing.T) {
			schedule, err := Parse(tc.spec)
			if err != nil {
				t.Fatalf("expected spec %q to parse, got error: %v", tc.spec, err)
			}

			next := schedule.Next(base)
			if got := next.Sub(base); got != tc.expected {
				t.Fatalf("expected interval %v, got %v", tc.expected, got)
			}
		})
	}
}

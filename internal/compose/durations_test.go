package compose

import (
	"testing"
	"time"
)

func TestDurationSecondsRoundedUp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		d    time.Duration
		want int
	}{
		{name: "zero", d: 0, want: 0},
		{name: "sub second", d: 500 * time.Millisecond, want: 1},
		{name: "one second", d: time.Second, want: 1},
		{name: "whole seconds", d: 5 * time.Second, want: 5},
		{name: "truncates fractional seconds above one", d: 1500 * time.Millisecond, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := durationSecondsRoundedUp(tt.d); got != tt.want {
				t.Fatalf("durationSecondsRoundedUp(%s) = %d, want %d", tt.d, got, tt.want)
			}
		})
	}
}

func TestParseDurationSecFallbackAndValidation(t *testing.T) {
	t.Parallel()

	if got, err := parseDurationSec("", 30); err != nil || got != 30 {
		t.Fatalf("parseDurationSec(empty) = %d, %v; want 30, nil", got, err)
	}
	if got, err := parseDurationSec("250ms", 30); err != nil || got != 1 {
		t.Fatalf("parseDurationSec(250ms) = %d, %v; want 1, nil", got, err)
	}
	if _, err := parseDurationSec("-1s", 30); err == nil {
		t.Fatal("parseDurationSec(-1s) error = nil, want error")
	}
}

func TestParseStopGracePeriodFallbackAndValidation(t *testing.T) {
	t.Parallel()

	if got, err := parseStopGracePeriod(""); err != nil || got != defaultStopGracePeriodSec {
		t.Fatalf("parseStopGracePeriod(empty) = %d, %v; want %d, nil", got, err, defaultStopGracePeriodSec)
	}
	if got, err := parseStopGracePeriod("250ms"); err != nil || got != 1 {
		t.Fatalf("parseStopGracePeriod(250ms) = %d, %v; want 1, nil", got, err)
	}
	if _, err := parseStopGracePeriod("-1s"); err == nil {
		t.Fatal("parseStopGracePeriod(-1s) error = nil, want error")
	}
}

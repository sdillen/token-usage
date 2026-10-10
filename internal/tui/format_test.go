package tui

import (
	"testing"
	"time"
)

func TestHumanTokens(t *testing.T) {
	for n, want := range map[int64]string{
		0: "0", 999: "999", 1_000: "1k", 12_345: "12.3k", 950_000: "950k",
		4_560_000: "4.56M", 478_775_702: "479M", 2_500_000_000: "2.5B",
	} {
		if got := humanTokens(n); got != want {
			t.Errorf("humanTokens(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestHumanDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		-time.Second: "now", 20 * time.Second: "<1m", 5 * time.Minute: "5m",
		2*time.Hour + 13*time.Minute: "2h 13m", 60 * time.Hour: "2d 12h",
	} {
		if got := humanDuration(d); got != want {
			t.Errorf("humanDuration(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestHumanCost(t *testing.T) {
	for c, want := range map[float64]string{0: "$0", 0.004: "<$0.01", 1.5: "$1.50", 250.4: "$250"} {
		if got := humanCost(c, false); got != want {
			t.Errorf("humanCost(%v) = %q, want %q", c, got, want)
		}
		if got := humanCost(c, true); got != "~"+want {
			t.Errorf("humanCost(%v, estimated) = %q, want %q", c, got, "~"+want)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello world", 6); got != "hello…" {
		t.Errorf("truncate = %q", got)
	}
	if got := truncate("hi", 6); got != "hi" {
		t.Errorf("truncate = %q", got)
	}
	if got := truncate("hi", 0); got != "" {
		t.Errorf("truncate = %q", got)
	}
}

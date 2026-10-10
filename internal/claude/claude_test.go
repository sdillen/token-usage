package claude

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sdillen/token-usage/internal/pricing"
	"github.com/sdillen/token-usage/internal/usage"
)

func TestPlanLabel(t *testing.T) {
	for _, tc := range []struct{ tier, sub, want string }{
		{"default_claude_max_5x", "max", "Max 5x"},
		{"default_claude_max_20x", "team", "Team 20x"},
		{"default_claude_max_5x", "", "Max 5x"},
		{"default", "pro", "Pro"},
		{"", "", ""},
	} {
		if got := planLabel(tc.tier, tc.sub); got != tc.want {
			t.Errorf("planLabel(%q, %q) = %q, want %q", tc.tier, tc.sub, got, tc.want)
		}
	}
}

func TestParseLimitsPercentScale(t *testing.T) {
	body := `{
	  "five_hour": {"utilization": 37.0, "resets_at": "2026-10-10T15:00:00Z"},
	  "seven_day": {"utilization": 1.0, "resets_at": "2026-10-13T12:00:00Z"},
	  "limits": [
	    {"kind": "weekly_scoped", "percent": 80, "resets_at": "2026-10-13T12:00:00Z", "scope": {"model": {"display_name": "Opus 5"}}},
	    {"kind": "weekly_scoped", "percent": 80, "scope": {"model": {"display_name": "Opus 5"}}},
	    {"kind": "weekly", "percent": 10}
	  ]}`
	limits, err := parseLimits([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		label   string
		percent float64
	}{{"Session (5h)", 0.37}, {"Weekly (7d)", 0.01}, {"Opus 5 Weekly", 0.8}}
	if len(limits) != len(want) {
		t.Fatalf("got %d limits, want %d: %+v", len(limits), len(want), limits)
	}
	for i, w := range want {
		if limits[i].Label != w.label || limits[i].Percent != w.percent {
			t.Errorf("limit %d = %+v, want %+v", i, limits[i], w)
		}
	}
	if limits[0].ResetsAt.IsZero() {
		t.Error("reset time not parsed")
	}
}

func TestParseLimitsFractions(t *testing.T) {
	limits, err := parseLimits([]byte(`{"five_hour": {"utilization": 0.37}, "seven_day": {"utilization": 0.5}}`))
	if err != nil || limits[0].Percent != 0.37 || limits[1].Percent != 0.5 {
		t.Errorf("limits = %+v, err = %v", limits, err)
	}
}

func TestParseLimitsEmpty(t *testing.T) {
	if _, err := parseLimits([]byte(`{}`)); err == nil {
		t.Error("expected an error for a payload without limits")
	}
}

func TestParseTime(t *testing.T) {
	want := time.Date(2026, 10, 10, 15, 0, 0, 0, time.UTC)
	for _, raw := range []string{`"2026-10-10T15:00:00Z"`, `1791644400`, `1791644400000`} {
		if got := parseTime([]byte(raw)); !got.Equal(want) {
			t.Errorf("parseTime(%s) = %v, want %v", raw, got, want)
		}
	}
	if !parseTime([]byte(`null`)).IsZero() {
		t.Error("null must parse to the zero time")
	}
}

const transcript = `{"type":"user","message":{"role":"user","content":"hi"}}
{"type":"assistant","uuid":"u1","sessionId":"s1","timestamp":"2026-10-10T10:00:00Z","message":{"id":"msg_1","role":"assistant","model":"claude-opus-5-5","usage":{"input_tokens":2,"output_tokens":1,"cache_read_input_tokens":100,"cache_creation_input_tokens":10}}}
{"type":"assistant","uuid":"u2","sessionId":"s1","timestamp":"2026-10-10T10:00:01Z","message":{"id":"msg_1","role":"assistant","model":"claude-opus-5-5","usage":{"input_tokens":2,"output_tokens":88,"cache_read_input_tokens":100,"cache_creation_input_tokens":10}}}
{"type":"assistant","uuid":"u3","sessionId":"s1","timestamp":"2026-10-10T10:01:00Z","message":{"role":"assistant","usage":{"input_tokens":0,"output_tokens":0}}}
not json "usage": {
`

func newTestCollector(t *testing.T, endpoint string) (*Collector, string) {
	t.Helper()
	dir := t.TempDir()
	projects := filepath.Join(dir, "projects", "p")
	if err := os.MkdirAll(projects, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projects, "s.jsonl"), []byte(transcript), 0o644); err != nil {
		t.Fatal(err)
	}
	exp := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	creds := `{"claudeAiOauth":{"accessToken":"tok","expiresAt":` + strconv.FormatInt(exp, 10) + `,"subscriptionType":"max","rateLimitTier":"default_claude_max_5x"}}`
	if err := os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte(creds), 0o600); err != nil {
		t.Fatal(err)
	}
	c := New(dir, pricing.Default())
	c.endpoint = endpoint
	c.cacheFile = filepath.Join(t.TempDir(), "limits.json")
	return c, dir
}

func TestCollect(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer tok" || r.Header.Get("anthropic-beta") == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"five_hour":{"utilization":42,"resets_at":"2099-01-01T00:00:00Z"}}`))
	}))
	defer srv.Close()

	c, _ := newTestCollector(t, srv.URL)
	c.now = func() time.Time { return time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC) }
	r := c.Collect(context.Background(), false)
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	if r.Plan != "Max 5x" {
		t.Errorf("Plan = %q", r.Plan)
	}
	// The streamed message counts once with the highest output count.
	if got := r.Summary.MonthMessages; got != 1 {
		t.Errorf("TotalMessages = %d, want 1", got)
	}
	if got := r.Summary.MonthTokens.Output; got != 88 {
		t.Errorf("Output = %d, want 88", got)
	}
	// 2 in, 88 out, 100 cache read, 10 cache write on claude-opus-5-5.
	if want := (2*4 + 88*20 + 100*0.2 + 10*5.0) / 1e6; !r.CostEstimated || math.Abs(r.Summary.MonthCost-want) > 1e-12 {
		t.Errorf("MonthCost = %v (estimated %v), want %v", r.Summary.MonthCost, r.CostEstimated, want)
	}
	if len(r.Limits) != 1 || r.Limits[0].Percent != 0.42 || r.LimitsNote != "" {
		t.Errorf("limits = %+v, note %q", r.Limits, r.LimitsNote)
	}

	// A second call within the throttle window is answered from the cache.
	c.Collect(context.Background(), false)
	if n := calls.Load(); n != 1 {
		t.Errorf("endpoint called %d times, want 1", n)
	}
	// Forcing bypasses the throttle.
	c.Collect(context.Background(), true)
	if n := calls.Load(); n != 2 {
		t.Errorf("endpoint called %d times after force, want 2", n)
	}
}

func TestCollectFallsBackToCacheOnRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "slow down", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c, _ := newTestCollector(t, srv.URL)
	now := time.Now()
	c.now = func() time.Time { return now }
	c.storeCache(limitsCache{FetchedAt: now.Add(-time.Hour), Limits: nil})
	if _, ok := c.loadCache(); ok {
		t.Fatal("empty cache must not load")
	}
	lim, err := parseLimits([]byte(`{"five_hour":{"utilization":10,"resets_at":"2099-01-01T00:00:00Z"}}`))
	if err != nil {
		t.Fatal(err)
	}
	c.storeCache(limitsCache{FetchedAt: now.Add(-time.Hour), Limits: lim})

	r := c.Collect(context.Background(), false)
	if len(r.Limits) != 1 || r.Limits[0].Percent != 0.1 {
		t.Errorf("limits = %+v", r.Limits)
	}
	if r.LimitsNote == "" {
		t.Error("stale cached limits must come with a note")
	}
}

func TestCollectWithoutLogin(t *testing.T) {
	c := New(t.TempDir(), pricing.Default())
	c.cacheFile = ""
	r := c.Collect(context.Background(), false)
	if r.Err != nil || len(r.Limits) != 0 || r.LimitsNote == "" {
		t.Errorf("report = %+v", r)
	}
}

func TestResetExpired(t *testing.T) {
	now := time.Now()
	got := resetExpired([]usage.Limit{{Percent: 0.5, ResetsAt: now.Add(-time.Minute)}, {Percent: 0.5, ResetsAt: now.Add(time.Hour)}}, now)
	if got[0].Percent != 0 || !got[0].ResetsAt.IsZero() {
		t.Errorf("expired window not reset: %+v", got[0])
	}
	if got[1].Percent != 0.5 {
		t.Errorf("open window changed: %+v", got[1])
	}
}

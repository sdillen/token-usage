package pi

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const session = `{"type":"session","id":"x"}
{"type":"message","id":"m1","timestamp":"2026-10-10T10:00:00.000Z","message":{"role":"user","content":"hi"}}
{"type":"message","id":"m2","timestamp":"2026-10-10T10:00:01.000Z","message":{"role":"assistant","provider":"FIT","model":"deepseek-v4","usage":{"input":120,"output":200,"cacheRead":12800,"cacheWrite":0,"totalTokens":13120,"cost":{"total":0.5}}}}
{"type":"message","id":"m3","timestamp":"2026-10-10T10:00:02.000Z","message":{"role":"assistant","provider":"anthropic","model":"claude-opus-5-5","usage":{"totalTokens":300,"cost":{"total":0.25}}}}
{"type":"message","id":"m4","timestamp":"2026-10-10T10:00:03.000Z","message":{"role":"assistant","usage":{"input":0,"output":0,"totalTokens":0}}}
`

func TestCollect(t *testing.T) {
	dir := t.TempDir()
	sessions := filepath.Join(dir, "sessions", "--proj--")
	if err := os.MkdirAll(sessions, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessions, "a.jsonl"), []byte(session), 0o644); err != nil {
		t.Fatal(err)
	}

	c := New(dir)
	c.now = func() time.Time { return time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC) }
	r := c.Collect(context.Background())
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	s := r.Summary
	if s.MonthMessages != 2 || s.MonthSessions != 1 {
		t.Errorf("messages/sessions = %d/%d, want 2/1", s.MonthMessages, s.MonthSessions)
	}
	if got := s.MonthTokens.Total(); got != 13120+300 {
		t.Errorf("total tokens = %d, want %d", got, 13120+300)
	}
	if s.MonthCost != 0.75 {
		t.Errorf("total cost = %v, want 0.75", s.MonthCost)
	}
	if len(s.Models) != 2 || s.Models[0].Model != "FIT/deepseek-v4" {
		t.Errorf("models = %+v", s.Models)
	}
}

func TestCollectWithoutSessions(t *testing.T) {
	r := New(t.TempDir()).Collect(context.Background())
	if r.Err != nil || r.Summary.MonthMessages != 0 {
		t.Errorf("report = %+v", r)
	}
}

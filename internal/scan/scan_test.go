package scan

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sdillen/token-usage/internal/usage"
)

func TestDedupeKeepsHighestOutput(t *testing.T) {
	in := []usage.Record{
		{Key: "m1", Tokens: usage.Tokens{Output: 1}},
		{Key: "m2", Tokens: usage.Tokens{Output: 5}},
		{Key: "m1", Tokens: usage.Tokens{Output: 90}},
		{Key: "m2", Tokens: usage.Tokens{Output: 2}},
	}
	out := Dedupe(in)
	if len(out) != 2 || out[0].Tokens.Output != 90 || out[1].Tokens.Output != 5 {
		t.Errorf("Dedupe = %+v", out)
	}
}

func TestScannerCachesUnchangedFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "a.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("one\ntwo\npartial")

	var parses atomic.Int32
	s := &Scanner{Roots: []string{dir, filepath.Join(dir, "missing")}, Parse: func(p string, l *LineReader) []usage.Record {
		parses.Add(1)
		var out []usage.Record
		for {
			line, ok := l.Next()
			if !ok {
				return out
			}
			out = append(out, usage.Record{Key: string(line)})
		}
	}}

	for i := 0; i < 2; i++ {
		got, err := s.Scan(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 { // the unterminated last line is still being written
			t.Fatalf("scan %d: %d records, want 2", i, len(got))
		}
	}
	if n := parses.Load(); n != 1 {
		t.Errorf("unchanged file parsed %d times, want 1", n)
	}

	write("one\ntwo\nthree\n")
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Scan(context.Background())
	if len(got) != 3 || parses.Load() != 2 {
		t.Errorf("after change: %d records, %d parses; want 3, 2", len(got), parses.Load())
	}
}

func TestScanHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := &Scanner{Roots: []string{t.TempDir()}, Parse: func(string, *LineReader) []usage.Record { return nil }}
	dir := s.Roots[0]
	if err := os.WriteFile(filepath.Join(dir, "a.jsonl"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Scan(ctx); err == nil {
		t.Error("Scan with cancelled context returned nil error")
	}
}

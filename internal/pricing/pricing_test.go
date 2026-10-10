package pricing

import (
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sdillen/token-usage/internal/usage"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestDefaultTable(t *testing.T) {
	tbl := Default()
	for model, want := range map[string]Price{
		"claude-opus-5-5":           {4, 20, 0.2, 5},
		"claude-opus-5-5-20260101":  {4, 20, 0.2, 5}, // snapshot matches by prefix
		"claude-sonnet-5-5":         {2, 10, 0.1, 2.5},
		"claude-haiku-5-5":          {0.1, 0.5, 0.01, 0.125},
		"claude-haiku-4-5":          {1, 5, 0.1, 1.25},
		"claude-haiku-4-5-20251001": {1, 5, 0.1, 1.25},
	} {
		got, ok := tbl.Lookup(model)
		if !ok || !reflect.DeepEqual(got, want) {
			t.Errorf("Lookup(%q) = %+v, %v; want %+v", model, got, ok, want)
		}
	}
	for _, model := range []string{"claude-sonnet-6", "claude-haiku-4-6", "gpt-5", ""} {
		if _, ok := tbl.Lookup(model); ok {
			t.Errorf("Lookup(%q) found a price, want none", model)
		}
	}
}

func TestCost(t *testing.T) {
	p := Price{Input: 4, Output: 20, CacheRead: 0.2, CacheWrite: 5}
	got := p.Cost(usage.Tokens{Input: 1_000_000, Output: 500_000, CacheRead: 10_000_000, CacheWrite: 200_000})
	if want := 4 + 10 + 2 + 1.0; !near(got, want) {
		t.Errorf("Cost = %v, want %v", got, want)
	}
}

func TestLoadOverridesAndExtends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prices.json")
	err := os.WriteFile(path, []byte(`{"models": [
		{"match": "claude-opus-5-5", "input": 1, "output": 2, "cache_read": 0.5, "cache_write": 3},
		{"match": "claude-sonnet-6", "input": 8, "output": 9, "cache_read": 1}
	]}`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	tbl, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if p, _ := tbl.Lookup("claude-opus-5-5"); p != (Price{1, 2, 0.5, 3}) {
		t.Errorf("override not applied: %+v", p)
	}
	if p, ok := tbl.Lookup("claude-sonnet-6"); !ok || p.Input != 8 || p.CacheWrite != 10 {
		t.Errorf("sonnet-6 = %+v, %v", p, ok)
	}
	if p, _ := tbl.Lookup("claude-sonnet-5-5"); p.Input != 2 {
		t.Errorf("embedded entries must survive: sonnet-5-5 = %+v", p)
	}
}

func TestLoadMissingFileIsFine(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.json")); err != nil {
		t.Errorf("Load of a missing file: %v", err)
	}
}

func TestLoadRejectsBadFiles(t *testing.T) {
	for name, content := range map[string]string{
		"not json":      `{`,
		"no match":      `{"models": [{"input": 1}]}`,
		"negative":      `{"models": [{"match": "x", "input": -1}]}`,
		"unknown field": `{"models": [{"match": "x", "inptu": 1}]}`,
	} {
		path := filepath.Join(t.TempDir(), "p.json")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "p.json") {
			t.Errorf("%s: err = %v, want an error naming the file", name, err)
		}
	}
}

func TestApply(t *testing.T) {
	records := []usage.Record{
		{Model: "claude-opus-5-5", Tokens: usage.Tokens{Input: 1_000_000}},
		{Model: "claude-sonnet-6", Tokens: usage.Tokens{Input: 1_000_000}},
		{Model: "claude-sonnet-6", Tokens: usage.Tokens{Input: 1}},
		{Model: "other", Cost: 7, Tokens: usage.Tokens{Input: 1}}, // already priced
	}
	unpriced := Default().Apply(records)
	if !reflect.DeepEqual(unpriced, []string{"claude-sonnet-6"}) {
		t.Errorf("unpriced = %v", unpriced)
	}
	if !near(records[0].Cost, 4) || records[1].Cost != 0 || records[3].Cost != 7 {
		t.Errorf("costs = %v %v %v", records[0].Cost, records[1].Cost, records[3].Cost)
	}
}

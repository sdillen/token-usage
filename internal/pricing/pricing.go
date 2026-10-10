// Package pricing estimates the cost of token usage from a table of list
// prices. The table is a JSON file embedded in the binary; users extend or
// override it with their own file, see Load.
package pricing

import (
	"bytes"
	_ "embed" // for the default price table
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"

	"github.com/sdillen/token-usage/internal/usage"
)

//go:embed prices.json
var defaultPrices []byte

// cacheWriteFactor is the cache-write price relative to the input price when
// a table entry does not state one (Anthropic's 5-minute cache).
const cacheWriteFactor = 1.25

// Price is a model's list price in USD per million tokens.
type Price struct {
	Input      float64
	Output     float64
	CacheRead  float64
	CacheWrite float64
}

// Cost returns the cost of t in USD.
func (p Price) Cost(t usage.Tokens) float64 {
	return (float64(t.Input)*p.Input +
		float64(t.Output)*p.Output +
		float64(t.CacheRead)*p.CacheRead +
		float64(t.CacheWrite)*p.CacheWrite) / 1e6
}

type entry struct {
	Match      string   `json:"match"`
	Input      float64  `json:"input"`
	Output     float64  `json:"output"`
	CacheRead  float64  `json:"cache_read"`
	CacheWrite *float64 `json:"cache_write"`
}

type file struct {
	Unit   string  `json:"unit"` // documentation for humans
	Note   string  `json:"note"`
	Models []entry `json:"models"`
}

// Table maps model ids to prices.
type Table struct {
	entries map[string]Price // keyed by match prefix
	order   []string         // prefixes, longest first
}

// Default returns the table embedded in the binary.
func Default() *Table {
	t := &Table{entries: map[string]Price{}}
	if err := t.merge(defaultPrices); err != nil {
		panic("pricing: embedded prices.json is invalid: " + err.Error()) // a build-time bug
	}
	return t
}

// Load returns the embedded table extended by the JSON file at path. Entries
// of the file replace embedded ones with the same "match". A missing file is
// not an error, so the path can point at the default location.
func Load(path string) (*Table, error) {
	t := Default()
	if path == "" {
		return t, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return t, nil
	}
	if err != nil {
		return nil, err
	}
	if err := t.merge(data); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return t, nil
}

func (t *Table) merge(data []byte) error {
	var f file
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return err
	}
	for _, e := range f.Models {
		if e.Match == "" {
			return errors.New(`model entry without "match"`)
		}
		if e.Input < 0 || e.Output < 0 || e.CacheRead < 0 {
			return fmt.Errorf("%s: negative price", e.Match)
		}
		p := Price{Input: e.Input, Output: e.Output, CacheRead: e.CacheRead, CacheWrite: e.Input * cacheWriteFactor}
		if e.CacheWrite != nil {
			p.CacheWrite = *e.CacheWrite
		}
		t.entries[e.Match] = p
	}
	t.order = t.order[:0]
	for m := range t.entries {
		t.order = append(t.order, m)
	}
	sort.Slice(t.order, func(i, j int) bool {
		if len(t.order[i]) != len(t.order[j]) {
			return len(t.order[i]) > len(t.order[j])
		}
		return t.order[i] < t.order[j]
	})
	return nil
}

// Lookup returns the price of model. The longest "match" that is a prefix of
// the model id wins, so "claude-opus-5-5" also covers dated snapshots such as
// "claude-opus-5-5-20260101".
func (t *Table) Lookup(model string) (Price, bool) {
	for _, m := range t.order {
		if strings.HasPrefix(model, m) {
			return t.entries[m], true
		}
	}
	return Price{}, false
}

// Apply fills in the cost of every record without one and returns the sorted
// models for which no price is known.
func (t *Table) Apply(records []usage.Record) (unpriced []string) {
	missing := map[string]struct{}{}
	for i, r := range records {
		if r.Cost > 0 {
			continue
		}
		p, ok := t.Lookup(r.Model)
		if !ok {
			missing[r.Model] = struct{}{}
			continue
		}
		records[i].Cost = p.Cost(r.Tokens)
	}
	for m := range missing {
		unpriced = append(unpriced, m)
	}
	sort.Strings(unpriced)
	return unpriced
}

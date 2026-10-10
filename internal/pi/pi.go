// Package pi collects usage of the pi coding agent from its session
// transcripts. All providers are included, as pi talks to many of them.
package pi

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/sdillen/token-usage/internal/scan"
	"github.com/sdillen/token-usage/internal/usage"
)

// Name is how the agent is displayed.
const Name = "Pi Coding Agent"

// Collector gathers the usage report for pi.
type Collector struct {
	scanner *scan.Scanner
	now     func() time.Time
}

// New returns a Collector reading sessions below dir. An empty dir resolves
// to $PI_CODING_AGENT_DIR or ~/.pi/agent.
func New(dir string) *Collector {
	if dir == "" {
		dir = os.Getenv("PI_CODING_AGENT_DIR")
	}
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".pi", "agent")
	}
	return &Collector{
		now: time.Now,
		scanner: &scan.Scanner{
			Roots: []string{filepath.Join(dir, "sessions")},
			Parse: parseSession,
		},
	}
}

// Collect builds the report.
func (c *Collector) Collect(ctx context.Context) usage.Report {
	now := c.now()
	report := usage.Report{Name: Name, UpdatedAt: now}
	records, err := c.scanner.Scan(ctx)
	if err != nil {
		report.Err = err
		return report
	}
	report.Summary = usage.Summarize(records, now)
	return report
}

type entry struct {
	Type      string    `json:"type"`
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Message   struct {
		Role     string `json:"role"`
		Provider string `json:"provider"`
		Model    string `json:"model"`
		Usage    *struct {
			Input      int64 `json:"input"`
			Output     int64 `json:"output"`
			CacheRead  int64 `json:"cacheRead"`
			CacheWrite int64 `json:"cacheWrite"`
			TotalToken int64 `json:"totalTokens"`
			Cost       struct {
				Total float64 `json:"total"`
			} `json:"cost"`
		} `json:"usage"`
	} `json:"message"`
}

var usageMarker = []byte(`"usage"`)

func parseSession(path string, lines *scan.LineReader) []usage.Record {
	var records []usage.Record
	n := 0
	for {
		raw, ok := lines.Next()
		if !ok {
			break
		}
		n++
		if !bytes.Contains(raw, usageMarker) {
			continue
		}
		var e entry
		if json.Unmarshal(raw, &e) != nil || e.Type != "message" || e.Message.Role != "assistant" || e.Message.Usage == nil {
			continue
		}
		u := e.Message.Usage
		tokens := usage.Tokens{Input: u.Input, Output: u.Output, CacheRead: u.CacheRead, CacheWrite: u.CacheWrite}
		if tokens.Total() <= 0 {
			// Only a grand total is known; count it as input.
			tokens = usage.Tokens{Input: u.TotalToken}
		}
		if tokens.Total() <= 0 {
			continue
		}
		key := path + ":" + e.ID
		if e.ID == "" {
			key = path + ":line" + strconv.Itoa(n)
		}
		model := e.Message.Model
		if model == "" {
			model = "unknown"
		}
		if e.Message.Provider != "" {
			model = e.Message.Provider + "/" + model
		}
		records = append(records, usage.Record{
			Key:     key,
			Model:   model,
			Session: path,
			Time:    e.Timestamp,
			Tokens:  tokens,
			Cost:    u.Cost.Total,
		})
	}
	return records
}

// Package claude collects Claude Code usage: token statistics from the local
// transcripts and the authoritative rate limits from Anthropic's OAuth usage
// endpoint.
package claude

import (
	"bytes"
	"encoding/json"
	"strconv"
	"time"

	"github.com/sdillen/token-usage/internal/scan"
	"github.com/sdillen/token-usage/internal/usage"
)

type line struct {
	Type      string    `json:"type"`
	UUID      string    `json:"uuid"`
	RequestID string    `json:"requestId"`
	SessionID string    `json:"sessionId"`
	Timestamp time.Time `json:"timestamp"`
	Message   struct {
		ID    string `json:"id"`
		Role  string `json:"role"`
		Model string `json:"model"`
		Usage *struct {
			Input      int64 `json:"input_tokens"`
			Output     int64 `json:"output_tokens"`
			CacheRead  int64 `json:"cache_read_input_tokens"`
			CacheWrite int64 `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

var usageMarker = []byte(`"usage":`)

// parseTranscript extracts the assistant messages with usage from one Claude
// Code transcript.
func parseTranscript(path string, lines *scan.LineReader) []usage.Record {
	var records []usage.Record
	n := 0
	for {
		raw, ok := lines.Next()
		if !ok {
			break
		}
		n++
		// Cheap pre-filter: most lines are unrelated to usage.
		if !bytes.Contains(raw, usageMarker) {
			continue
		}
		var l line
		if json.Unmarshal(raw, &l) != nil || l.Message.Usage == nil {
			continue
		}
		if l.Type != "assistant" && l.Message.Role != "assistant" {
			continue
		}
		u := l.Message.Usage
		tokens := usage.Tokens{Input: u.Input, Output: u.Output, CacheRead: u.CacheRead, CacheWrite: u.CacheWrite}
		if tokens.Total() <= 0 {
			continue
		}
		key := l.Message.ID
		if key == "" {
			key = path + ":" + firstNonEmpty(l.UUID, l.RequestID, strconv.Itoa(n))
		}
		model := l.Message.Model
		if model == "" {
			model = "claude"
		}
		records = append(records, usage.Record{
			Key:     key,
			Model:   model,
			Session: firstNonEmpty(l.SessionID, path),
			Time:    l.Timestamp,
			Tokens:  tokens,
		})
	}
	return records
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

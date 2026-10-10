package scan

import "github.com/sdillen/token-usage/internal/usage"

// Dedupe keeps one record per key. A streamed response is written as several
// lines sharing one message id, where earlier lines carry placeholder output
// counts, so the record with the highest output count wins (the later one on
// a tie). Order of first appearance is preserved.
func Dedupe(records []usage.Record) []usage.Record {
	pos := make(map[string]int, len(records))
	out := make([]usage.Record, 0, len(records))
	for _, r := range records {
		i, seen := pos[r.Key]
		switch {
		case !seen:
			pos[r.Key] = len(out)
			out = append(out, r)
		case r.Tokens.Output >= out[i].Tokens.Output:
			out[i] = r
		}
	}
	return out
}

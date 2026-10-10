// Package usage holds the agent-independent data model: raw token records,
// their aggregation into display-ready summaries and rate-limit windows.
package usage

import (
	"sort"
	"time"
)

// RecentDays is the number of days shown in the history chart.
const RecentDays = 7

// Tokens is a token count split by kind.
type Tokens struct {
	Input      int64
	Output     int64
	CacheRead  int64
	CacheWrite int64
}

// Total returns the sum of all token kinds.
func (t Tokens) Total() int64 {
	return t.Input + t.Output + t.CacheRead + t.CacheWrite
}

// Add returns the component-wise sum of t and o.
func (t Tokens) Add(o Tokens) Tokens {
	return Tokens{
		Input:      t.Input + o.Input,
		Output:     t.Output + o.Output,
		CacheRead:  t.CacheRead + o.CacheRead,
		CacheWrite: t.CacheWrite + o.CacheWrite,
	}
}

// Record is a single assistant message with its token usage.
type Record struct {
	Key     string // unique message key, used for de-duplication
	Model   string
	Session string
	Time    time.Time
	Tokens  Tokens
	Cost    float64 // USD, 0 when the agent does not report costs
}

// Day is the token total of one local calendar day.
type Day struct {
	Date   time.Time
	Tokens int64
}

// ModelStat aggregates all records of one model.
type ModelStat struct {
	Model    string
	Tokens   Tokens
	Messages int
	Cost     float64
}

// Summary is the aggregated usage of one agent, or of several merged.
type Summary struct {
	TodayTokens   Tokens
	TodayMessages int
	TodaySessions int
	TodayCost     float64

	Recent []Day // oldest first, always RecentDays entries

	MonthTokens   Tokens
	MonthMessages int
	MonthSessions int
	MonthCost     float64
	MonthDays     []Day       // every day of the month, first to last
	Models        []ModelStat // models used this month, by total tokens, descending
}

// Limit is one rate-limit window, e.g. the 5-hour session limit.
type Limit struct {
	Label    string
	Percent  float64   // 0..1
	ResetsAt time.Time // zero when unknown
}

// Part is one agent's share of a merged report.
type Part struct {
	Name   string
	Tokens int64 // this month
}

// Report is everything the UI shows for one agent.
type Report struct {
	Name    string
	Plan    string
	Summary Summary
	Limits  []Limit
	Parts   []Part // per-agent month totals, only set on merged reports
	// CostEstimated is set when costs are computed from list prices rather
	// than reported by the agent. Unpriced lists the models that have no
	// price and are therefore missing from the costs.
	CostEstimated bool
	Unpriced      []string
	LimitsNote    string // why limits are missing or stale, if they are
	Err           error
	UpdatedAt     time.Time
}

// Summarize aggregates records into a Summary relative to now. Days and the
// month are evaluated in now's location.
func Summarize(records []Record, now time.Time) Summary {
	loc := now.Location()
	today := dayStart(now)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	daysInMonth := monthStart.AddDate(0, 1, -1).Day()

	s := Summary{Recent: make([]Day, RecentDays), MonthDays: make([]Day, daysInMonth)}
	recent := make(map[time.Time]int, RecentDays)
	for i := range s.Recent {
		d := today.AddDate(0, 0, i-(RecentDays-1))
		s.Recent[i].Date = d
		recent[d] = i
	}
	for i := range s.MonthDays {
		s.MonthDays[i].Date = monthStart.AddDate(0, 0, i)
	}

	todaySessions := map[string]struct{}{}
	monthSessions := map[string]struct{}{}
	models := map[string]*ModelStat{}

	for _, r := range records {
		t := r.Time.In(loc)
		day := dayStart(t)
		total := r.Tokens.Total()

		if i, ok := recent[day]; ok {
			s.Recent[i].Tokens += total
		}
		if day.Equal(today) {
			s.TodayMessages++
			s.TodayTokens = s.TodayTokens.Add(r.Tokens)
			s.TodayCost += r.Cost
			todaySessions[r.Session] = struct{}{}
		}
		if t.Year() == now.Year() && t.Month() == now.Month() {
			s.MonthMessages++
			s.MonthTokens = s.MonthTokens.Add(r.Tokens)
			s.MonthCost += r.Cost
			s.MonthDays[t.Day()-1].Tokens += total
			monthSessions[r.Session] = struct{}{}

			m := models[r.Model]
			if m == nil {
				m = &ModelStat{Model: r.Model}
				models[r.Model] = m
			}
			m.Tokens = m.Tokens.Add(r.Tokens)
			m.Messages++
			m.Cost += r.Cost
		}
	}

	s.TodaySessions = len(todaySessions)
	s.MonthSessions = len(monthSessions)
	for _, m := range models {
		s.Models = append(s.Models, *m)
	}
	sortModels(s.Models)
	return s
}

// Merge adds up summaries of agents that were computed for the same now.
// Sessions are never shared between agents, so their counts simply add up.
func Merge(parts ...Summary) Summary {
	var out Summary
	models := map[string]*ModelStat{}
	for _, p := range parts {
		out.TodayTokens = out.TodayTokens.Add(p.TodayTokens)
		out.TodayMessages += p.TodayMessages
		out.TodaySessions += p.TodaySessions
		out.TodayCost += p.TodayCost
		out.MonthTokens = out.MonthTokens.Add(p.MonthTokens)
		out.MonthMessages += p.MonthMessages
		out.MonthSessions += p.MonthSessions
		out.MonthCost += p.MonthCost
		out.Recent = addDays(out.Recent, p.Recent)
		out.MonthDays = addDays(out.MonthDays, p.MonthDays)
		for _, m := range p.Models {
			acc := models[m.Model]
			if acc == nil {
				acc = &ModelStat{Model: m.Model}
				models[m.Model] = acc
			}
			acc.Tokens = acc.Tokens.Add(m.Tokens)
			acc.Messages += m.Messages
			acc.Cost += m.Cost
		}
	}
	for _, m := range models {
		out.Models = append(out.Models, *m)
	}
	sortModels(out.Models)
	return out
}

// addDays adds the per-day totals of b to a copy of a.
func addDays(a, b []Day) []Day {
	if a == nil {
		return append([]Day(nil), b...)
	}
	out := append([]Day(nil), a...)
	for i := range out {
		if i < len(b) {
			out[i].Tokens += b[i].Tokens
		}
	}
	return out
}

func sortModels(models []ModelStat) {
	sort.Slice(models, func(i, j int) bool {
		a, b := models[i].Tokens.Total(), models[j].Tokens.Total()
		if a != b {
			return a > b
		}
		return models[i].Model < models[j].Model
	})
}

func dayStart(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

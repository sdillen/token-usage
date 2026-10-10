package usage

import (
	"testing"
	"time"
)

func rec(now time.Time, key, model, session string, daysAgo int, in, out int64) Record {
	return Record{Key: key, Model: model, Session: session, Time: now.AddDate(0, 0, -daysAgo), Tokens: Tokens{Input: in, Output: out}, Cost: 0.5}
}

func TestSummarize(t *testing.T) {
	now := time.Date(2026, 10, 10, 15, 0, 0, 0, time.UTC)
	s := Summarize([]Record{
		rec(now, "a", "m1", "s1", 0, 100, 10),
		rec(now, "b", "m1", "s1", 0, 50, 5),
		rec(now, "c", "m2", "s2", 2, 1000, 0),
		rec(now, "d", "m2", "s3", 30, 7, 0), // September: outside the month
	}, now)

	if got := s.TodayTokens.Total(); got != 165 {
		t.Errorf("TodayTokens = %d, want 165", got)
	}
	if s.TodayMessages != 2 || s.TodaySessions != 1 {
		t.Errorf("today messages/sessions = %d/%d, want 2/1", s.TodayMessages, s.TodaySessions)
	}
	if got := s.Recent[RecentDays-1].Tokens; got != 165 {
		t.Errorf("Recent[today] = %d, want 165", got)
	}
	if got := s.Recent[RecentDays-3].Tokens; got != 1000 {
		t.Errorf("Recent[today-2] = %d, want 1000", got)
	}
	if got := s.MonthTokens.Total(); got != 1165 {
		t.Errorf("MonthTokens = %d, want 1165", got)
	}
	if s.MonthMessages != 3 || s.MonthSessions != 2 || s.MonthCost != 1.5 {
		t.Errorf("month messages/sessions/cost = %d/%d/%v, want 3/2/1.5", s.MonthMessages, s.MonthSessions, s.MonthCost)
	}
	if len(s.MonthDays) != 31 || s.MonthDays[9].Tokens != 165 || s.MonthDays[7].Tokens != 1000 {
		t.Errorf("MonthDays wrong: len %d, day10 %d, day8 %d", len(s.MonthDays), s.MonthDays[9].Tokens, s.MonthDays[7].Tokens)
	}
	if len(s.Models) != 2 || s.Models[0].Model != "m2" || s.Models[0].Tokens.Total() != 1000 {
		t.Errorf("Models = %+v", s.Models)
	}
}

func TestSummarizeEmpty(t *testing.T) {
	s := Summarize(nil, time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC))
	if len(s.Recent) != RecentDays || len(s.MonthDays) != 28 || s.MonthMessages != 0 || len(s.Models) != 0 {
		t.Errorf("unexpected empty summary: %+v", s)
	}
}

func TestMerge(t *testing.T) {
	now := time.Date(2026, 10, 10, 15, 0, 0, 0, time.UTC)
	a := Summarize([]Record{rec(now, "a", "m1", "s1", 0, 100, 0), rec(now, "b", "shared", "s1", 1, 10, 0)}, now)
	b := Summarize([]Record{rec(now, "c", "shared", "s2", 0, 5, 0)}, now)
	m := Merge(a, b)

	if m.TodayTokens.Total() != 105 || m.TodayMessages != 2 || m.TodaySessions != 2 {
		t.Errorf("today = %+v", m)
	}
	if m.MonthTokens.Total() != 115 || m.MonthSessions != 2 || m.MonthCost != 1.5 {
		t.Errorf("month = %d tokens, %d sessions, %v cost", m.MonthTokens.Total(), m.MonthSessions, m.MonthCost)
	}
	if m.Recent[RecentDays-1].Tokens != 105 || m.MonthDays[9].Tokens != 105 || m.MonthDays[8].Tokens != 10 {
		t.Errorf("days not summed: %+v", m.Recent)
	}
	if len(m.Models) != 2 || m.Models[0].Model != "m1" || m.Models[1].Tokens.Total() != 15 {
		t.Errorf("models = %+v", m.Models)
	}
	if len(Merge().Recent) != 0 {
		t.Error("Merge() of nothing must be empty")
	}
}

package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/sdillen/token-usage/internal/usage"
)

const (
	// DefaultEndpoint is Anthropic's OAuth usage endpoint.
	DefaultEndpoint = "https://api.anthropic.com/api/oauth/usage"

	// minProbeInterval keeps the usage endpoint, which rate-limits readily,
	// from being asked more often than the numbers change.
	minProbeInterval = 60 * time.Second
	// staleAfter is how old cached numbers may get before they are reported
	// as stale after a failed probe.
	staleAfter = 15 * time.Minute
)

// credentials is the part of ~/.claude/.credentials.json that is needed here.
type credentials struct {
	AccessToken      string
	ExpiresAt        time.Time
	SubscriptionType string
	RateLimitTier    string
}

// readCredentials prefers the credentials file. On macOS Claude Code keeps them
// in the Keychain instead, which is consulted only for the default config dir:
// the Keychain entry name does not carry the dir, so a custom dir would
// otherwise pick up another account's token.
func readCredentials(dir string) (credentials, error) {
	data, err := os.ReadFile(filepath.Join(dir, ".credentials.json"))
	if err != nil {
		home, _ := os.UserHomeDir()
		if runtime.GOOS != "darwin" || dir != filepath.Join(home, ".claude") {
			return credentials{}, err
		}
		if data, err = keychainCredentials(); err != nil {
			return credentials{}, err
		}
	}
	return parseCredentials(data)
}

// keychainCredentials returns the JSON Claude Code stores in the macOS Keychain.
var keychainCredentials = func() ([]byte, error) {
	return exec.Command("security", "find-generic-password", "-s", "Claude Code-credentials", "-w").Output()
}

func parseCredentials(data []byte) (credentials, error) {
	var file struct {
		OAuth *struct {
			AccessToken      string `json:"accessToken"`
			ExpiresAt        int64  `json:"expiresAt"`
			SubscriptionType string `json:"subscriptionType"`
			RateLimitTier    string `json:"rateLimitTier"`
		} `json:"claudeAiOauth"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return credentials{}, err
	}
	if file.OAuth == nil || file.OAuth.AccessToken == "" {
		return credentials{}, errors.New("no OAuth login found")
	}
	o := file.OAuth
	return credentials{
		AccessToken:      o.AccessToken,
		ExpiresAt:        time.UnixMilli(o.ExpiresAt),
		SubscriptionType: o.SubscriptionType,
		RateLimitTier:    o.RateLimitTier,
	}, nil
}

// planLabel names the plan, e.g. "Max 5x". The tier names the ceiling, the
// subscription the plan, so a Team seat on a Max-sized tier reads "Team 5x".
func planLabel(tier, subscription string) string {
	plan := ""
	if subscription != "" {
		plan = strings.ToUpper(subscription[:1]) + subscription[1:]
	}
	lower := strings.ToLower(tier)
	i := strings.Index(lower, "max_")
	if i < 0 {
		return plan
	}
	mult := lower[i+len("max_"):]
	if j := strings.IndexFunc(mult, func(r rune) bool { return r == '_' || r == ' ' }); j >= 0 {
		mult = mult[:j]
	}
	if !strings.HasSuffix(mult, "x") {
		return plan
	}
	if plan == "" {
		plan = "Max"
	}
	return plan + " " + mult
}

// profileTier reads the rate-limit tier from the CLI's profile. The tier in
// the credentials is stamped when the token is minted and not updated on an
// upgrade, so the profile, which is refreshed against the account, wins when
// it names a multiplier.
func profileTier(dir string) string {
	candidates := []string{filepath.Join(dir, ".config.json"), filepath.Join(dir, ".claude.json")}
	if home, err := os.UserHomeDir(); err == nil && dir == filepath.Join(home, ".claude") {
		candidates = append(candidates, filepath.Join(home, ".claude.json"))
	}
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var p struct {
			Account *struct {
				User string `json:"userRateLimitTier"`
				Org  string `json:"organizationRateLimitTier"`
			} `json:"oauthAccount"`
		}
		if json.Unmarshal(data, &p) != nil || p.Account == nil {
			continue
		}
		return firstNonEmpty(p.Account.User, p.Account.Org)
	}
	return ""
}

func hasMultiplier(tier string) bool {
	return planLabel(tier, "x") != "X"
}

type bucket struct {
	Utilization json.RawMessage `json:"utilization"`
	ResetsAt    json.RawMessage `json:"resets_at"`
}

type usagePayload struct {
	FiveHour         *bucket `json:"five_hour"`
	SevenDay         *bucket `json:"seven_day"`
	SevenDayOAuthApp *bucket `json:"seven_day_oauth_apps"`
	Limits           []struct {
		Kind    string          `json:"kind"`
		Percent json.RawMessage `json:"percent"`
		Resets  json.RawMessage `json:"resets_at"`
		Scope   *struct {
			Model *struct {
				ID          string `json:"id"`
				DisplayName string `json:"display_name"`
			} `json:"model"`
		} `json:"scope"`
	} `json:"limits"`
}

// parseLimits converts the endpoint's payload into limit windows: the session
// and weekly buckets plus every model-scoped window from the limits array.
func parseLimits(body []byte) ([]usage.Limit, error) {
	var p usagePayload
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	weekly := p.SevenDayOAuthApp
	if weekly == nil {
		weekly = p.SevenDay
	}

	// One payload speaks one convention: percentages (37.0) or, in older
	// payloads, fractions (0.37). Any value >= 1 settles it for all.
	var raws []json.RawMessage
	for _, b := range []*bucket{p.FiveHour, weekly} {
		if b != nil {
			raws = append(raws, b.Utilization)
		}
	}
	for _, e := range p.Limits {
		raws = append(raws, e.Percent)
	}
	percentScale := false
	for _, r := range raws {
		if v, ok := parseNumber(r); ok && v >= 1 {
			percentScale = true
		}
	}

	var limits []usage.Limit
	add := func(label string, util, reset json.RawMessage) {
		v, ok := parseNumber(util)
		if !ok || v < 0 {
			return
		}
		if percentScale || v > 1 {
			v /= 100
		}
		limits = append(limits, usage.Limit{Label: label, Percent: math.Min(v, 1), ResetsAt: parseTime(reset)})
	}
	if p.FiveHour != nil {
		add("Session (5h)", p.FiveHour.Utilization, p.FiveHour.ResetsAt)
	}
	if weekly != nil {
		add("Weekly (7d)", weekly.Utilization, weekly.ResetsAt)
	}

	seen := map[string]bool{}
	for _, e := range p.Limits {
		if e.Scope == nil || e.Scope.Model == nil {
			continue
		}
		name := strings.TrimSpace(firstNonEmpty(e.Scope.Model.DisplayName, e.Scope.Model.ID))
		if name == "" {
			continue
		}
		label := strings.TrimSpace(name + " " + scopedWindow(e.Kind))
		if seen[label] {
			continue
		}
		seen[label] = true
		add(label, e.Percent, e.Resets)
	}

	if len(limits) == 0 {
		return nil, errors.New("usage endpoint returned no limits")
	}
	return limits, nil
}

func scopedWindow(kind string) string {
	k := strings.ToLower(kind)
	switch {
	case strings.Contains(k, "month"):
		return "Monthly"
	case strings.Contains(k, "week"), strings.Contains(k, "day"):
		return "Weekly"
	case strings.Contains(k, "hour"), strings.Contains(k, "session"):
		return "Session"
	}
	return ""
}

// parseNumber accepts JSON numbers as well as strings such as "37%".
func parseNumber(raw json.RawMessage) (float64, bool) {
	s := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	s = strings.TrimSuffix(s, "%")
	if s == "" || s == "null" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) {
		return 0, false
	}
	return v, true
}

// parseTime accepts RFC 3339 strings and Unix timestamps in seconds or
// milliseconds.
func parseTime(raw json.RawMessage) time.Time {
	s := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if s == "" || s == "null" {
		return time.Time{}
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		if n < 1e12 {
			return time.Unix(n, 0)
		}
		return time.UnixMilli(n)
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// limitsCache is the on-disk copy of the last successful probe. It survives
// restarts so that starting the TUI repeatedly does not hammer the endpoint.
type limitsCache struct {
	FetchedAt time.Time     `json:"fetchedAt"`
	Limits    []usage.Limit `json:"limits"`
}

func (c *Collector) loadCache() (limitsCache, bool) {
	if c.cacheFile == "" {
		return limitsCache{}, false
	}
	data, err := os.ReadFile(c.cacheFile)
	if err != nil {
		return limitsCache{}, false
	}
	var lc limitsCache
	if json.Unmarshal(data, &lc) != nil || len(lc.Limits) == 0 {
		return limitsCache{}, false
	}
	return lc, true
}

func (c *Collector) storeCache(lc limitsCache) {
	if c.cacheFile == "" {
		return
	}
	data, err := json.Marshal(lc)
	if err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(c.cacheFile), 0o700) != nil {
		return
	}
	tmp := c.cacheFile + ".tmp"
	if os.WriteFile(tmp, data, 0o600) != nil {
		return
	}
	_ = os.Rename(tmp, c.cacheFile)
}

// resetExpired zeroes windows whose period ended since they were measured.
func resetExpired(limits []usage.Limit, now time.Time) []usage.Limit {
	out := make([]usage.Limit, len(limits))
	for i, l := range limits {
		if !l.ResetsAt.IsZero() && !l.ResetsAt.After(now) {
			l.Percent, l.ResetsAt = 0, time.Time{}
		}
		out[i] = l
	}
	return out
}

// limits returns the rate limits, probing the endpoint at most once per
// minProbeInterval. If the probe fails, cached numbers are returned with a
// note instead.
func (c *Collector) limits(ctx context.Context, creds credentials, force bool) ([]usage.Limit, string) {
	now := c.now()
	cached, hasCache := c.loadCache()

	if hasCache && !force && now.Sub(cached.FetchedAt) < minProbeInterval {
		return resetExpired(cached.Limits, now), ""
	}

	limits, err := c.probe(ctx, creds, now)
	if err == nil {
		c.storeCache(limitsCache{FetchedAt: now, Limits: limits})
		return limits, ""
	}
	if hasCache {
		note := ""
		if age := now.Sub(cached.FetchedAt); age > staleAfter {
			note = fmt.Sprintf("limits are %s old: %v", age.Round(time.Minute), err)
		}
		return resetExpired(cached.Limits, now), note
	}
	return nil, err.Error()
}

func (c *Collector) probe(ctx context.Context, creds credentials, now time.Time) ([]usage.Limit, error) {
	if !creds.ExpiresAt.IsZero() && creds.ExpiresAt.Before(now) {
		return nil, errors.New("OAuth token expired, start `claude` once to refresh it")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+creds.AccessToken)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.New("could not reach Anthropic's usage endpoint")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, errors.New("usage endpoint is rate limiting checks")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("usage endpoint returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	return parseLimits(body)
}

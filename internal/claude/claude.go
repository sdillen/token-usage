package claude

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/sdillen/token-usage/internal/pricing"
	"github.com/sdillen/token-usage/internal/scan"
	"github.com/sdillen/token-usage/internal/usage"
)

// Name is how the agent is displayed.
const Name = "Claude Code"

// Collector gathers the usage report for Claude Code.
type Collector struct {
	dir       string
	endpoint  string
	cacheFile string
	client    *http.Client
	now       func() time.Time
	scanner   *scan.Scanner
	prices    *pricing.Table
}

// New returns a Collector for the Claude config directory dir. An empty dir
// resolves to $CLAUDE_CONFIG_DIR or ~/.claude. Costs are estimated with prices.
func New(dir string, prices *pricing.Table) *Collector {
	if dir == "" {
		dir = os.Getenv("CLAUDE_CONFIG_DIR")
	}
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".claude")
	}
	cacheFile := ""
	if cache, err := os.UserCacheDir(); err == nil {
		cacheFile = filepath.Join(cache, "token-usage", "claude-limits.json")
	}
	return &Collector{
		dir:       dir,
		endpoint:  DefaultEndpoint,
		cacheFile: cacheFile,
		client:    &http.Client{},
		prices:    prices,
		now:       time.Now,
		scanner: &scan.Scanner{
			Roots: []string{filepath.Join(dir, "projects")},
			Parse: parseTranscript,
		},
	}
}

// Collect builds the report. force bypasses the limits probe throttle.
func (c *Collector) Collect(ctx context.Context, force bool) usage.Report {
	now := c.now()
	report := usage.Report{Name: Name, UpdatedAt: now}

	records, err := c.scanner.Scan(ctx)
	if err != nil {
		report.Err = err
		return report
	}
	records = scan.Dedupe(records)
	report.Unpriced = c.prices.Apply(records)
	report.CostEstimated = true
	report.Summary = usage.Summarize(records, now)

	creds, err := readCredentials(c.dir)
	if err != nil {
		report.LimitsNote = "no Claude login found, run `claude auth login` for limits"
		return report
	}
	tier := profileTier(c.dir)
	if !hasMultiplier(tier) {
		tier = creds.RateLimitTier
	}
	report.Plan = planLabel(tier, creds.SubscriptionType)
	report.Limits, report.LimitsNote = c.limits(ctx, creds, force)
	return report
}

// Command token-usage shows the token usage and rate limits of Claude Code
// and pi in a terminal UI.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sdillen/token-usage/internal/claude"
	"github.com/sdillen/token-usage/internal/pi"
	"github.com/sdillen/token-usage/internal/pricing"
	"github.com/sdillen/token-usage/internal/tui"
	"github.com/sdillen/token-usage/internal/usage"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = ""

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "token-usage:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		asJSON      = flag.Bool("json", false, "print the usage reports once as JSON and exit")
		showVersion = flag.Bool("version", false, "print the version and exit")
		claudeDir   = flag.String("claude-dir", "", "Claude config directory (default $CLAUDE_CONFIG_DIR or ~/.claude)")
		pricesFile  = flag.String("prices", "", "extra price table (default "+defaultPricesPath()+")")
		piDir       = flag.String("pi-dir", "", "pi agent directory (default $PI_CODING_AGENT_DIR or ~/.pi/agent)")
	)
	flag.Parse()

	if *showVersion {
		fmt.Println("token-usage", buildVersion())
		return nil
	}

	path := *pricesFile
	if path == "" {
		path = defaultPricesPath()
	}
	prices, err := pricing.Load(path)
	if err != nil {
		return fmt.Errorf("price table: %w", err)
	}

	cc := claude.New(*claudeDir, prices)
	pc := pi.New(*piDir)
	sources := []tui.Source{
		{Name: claude.Name, Collect: cc.Collect},
		{Name: pi.Name, Collect: func(ctx context.Context, _ bool) usage.Report { return pc.Collect(ctx) }},
	}

	if *asJSON {
		reports := make([]usage.Report, len(sources))
		for i, s := range sources {
			reports[i] = s.Collect(context.Background(), true)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(reports)
	}

	_, err = tea.NewProgram(tui.New(sources), tea.WithAltScreen()).Run()
	return err
}

// defaultPricesPath is where user-defined prices are read from, if present.
func defaultPricesPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "token-usage", "prices.json")
}

func buildVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "dev"
}

# token-usage

A terminal UI that shows how much room is left in your Claude subscription and
what you have spent on [Claude Code](https://claude.com/claude-code) and
[pi](https://github.com/badlogic/pi-mono). Inspired by the agents panel of
[omarchy](https://github.com/basecamp/omarchy), built with
[Bubble Tea](https://github.com/charmbracelet/bubbletea) and
[Lip Gloss](https://github.com/charmbracelet/lipgloss).

Three tabs: **Total** (Claude Code and pi summed up, with the share of each
agent), **Claude Code** and **pi**. Each tab shows:

- **Limits** (Claude Code): the 5-hour session window, the weekly window and
  model-scoped windows, with a countdown to the reset. Read from Anthropic's
  OAuth usage endpoint with the login Claude Code already stores.
- **Today**: tokens (in / out / cache), messages and sessions.
- **Last 7 days**: a daily token chart.
- **This month**: totals, cost (where the agent reports it), a per-day chart
  and the share of every model.

All token statistics come from the local session transcripts; nothing leaves
your machine except the single request for the Claude limits.

## Install

```sh
go install github.com/sdillen/token-usage/cmd/token-usage@latest
```

Or download a binary from the [releases](https://github.com/sdillen/token-usage/releases).

## Usage

```sh
token-usage            # start the TUI
token-usage -json      # print the reports once as JSON
token-usage -version
```

| Key       | Action                     |
| --------- | -------------------------- |
| `tab` / `←` `→` / `1`–`3` | switch tab    |
| `r`       | refresh now                |
| `↑` / `↓` | scroll (small terminals)   |
| `q`       | quit                       |

The data refreshes itself every 30 seconds. The Claude limits are queried at
most once a minute and cached in `$XDG_CACHE_HOME/token-usage`, because
Anthropic rate-limits the endpoint.

### Options

| Flag           | Default                                  | Description               |
| -------------- | ---------------------------------------- | ------------------------- |
| `-claude-dir`  | `$CLAUDE_CONFIG_DIR` or `~/.claude`      | Claude Code config dir    |
| `-prices`      | `~/.config/token-usage/prices.json`      | Extra price table         |
| `-pi-dir`      | `$PI_CODING_AGENT_DIR` or `~/.pi/agent`  | pi agent directory        |

### Cost estimates

Claude Code does not record what a message cost, so it is estimated from list
prices. The built-in table is
[`internal/pricing/prices.json`](internal/pricing/prices.json), with prices in
USD per million tokens. Estimates are shown with a leading `~`. Models without a
price are listed in the card and left out of the cost.

To add or change models, create `~/.config/token-usage/prices.json` (or pass
`-prices FILE`). Its entries extend the built-in table and replace entries with
the same `match`:

```json
{
  "models": [
    {"match": "claude-sonnet-6", "input": 3.00, "output": 15.00, "cache_read": 0.30}
  ]
}
```

`match` is a prefix of the model id, and the longest match wins, so
`claude-opus-5-5` also covers dated snapshots. `cache_write` is optional and
defaults to 1.25 × `input`. pi reports its own costs, which are used as they are.

## Development

The toolchain comes from [devenv](https://devenv.sh); with direnv it loads
automatically.

```sh
make help    # list all targets
make test    # tests with the race detector
make lint    # golangci-lint
make check   # everything CI checks
make build   # binary in ./bin
```

Releases are built by GoReleaser when a `v*` tag is pushed.

## License

[MIT](LICENSE)

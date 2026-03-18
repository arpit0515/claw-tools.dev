# 🦞 ClawTools

**MCP tool connectors for AI agents. Clone the repo, pick your tools, run on your own hardware.**

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Website](https://img.shields.io/badge/website-clawtools.dev-orange)](https://claw-tools.dev)

---

## What is this?

ClawTools is a collection of MCP (Model Context Protocol) tool connectors that let any AI agent - PicoClaw, Cursor, Claude Code, Windsurf, or your own - connect to real services like Gmail, Google Calendar, and Outlook.

- **No hosted infrastructure.** Runs entirely on your machine or Pi.
- **Your tokens stay yours.** OAuth tokens saved in `~/.picoclaw/tokens/` - never sent anywhere.
- **Any agent.** MCP stdio for editors. HTTP REST for custom agents.
- **Open source.** MIT licensed. Add a tool via PR.

---

## Repo structure

```
claw-tools.dev/
├── install.sh              ← Run this to get started
├── tools.json              ← Registry of all available tools
├── cmd/
│   └── installer/
│       ├── main.go         ← Interactive TUI installer (Go)
│       └── go.mod
└── tools/
    ├── gmail/              ← Gmail connector
    │   ├── main.go
    │   └── go.mod
    ├── gcal/               ← Google Calendar connector
    │   ├── main.go
    │   └── go.mod
    └── _template/          ← Copy this to build a new tool
        ├── main.go
        └── go.mod
```

---

## Quick start

```bash
git clone https://github.com/arpit0515/claw-tools.dev
cd claw-tools.dev
./install.sh
```

The interactive installer picks up the `tools.json` at the root, shows all available tools, and sets up only what you choose:

```
  ClawTools Installer
  ──────────────────────────────────────
  ● Available

  [ ] 1  Gmail Connector
         Read and search Gmail. Powers email summaries.
         gmail_list  gmail_search  gmail_get

  [ ] 2  Google Calendar
         Today's schedule and upcoming events.
         gcal_today  gcal_upcoming  gcal_get

  ○ Coming soon
  [ ] 3  Outlook / Exchange - Microsoft Graph API connector
  ──────────────────────────────────────
  → no tools selected yet

  ❯
```

Type numbers to toggle, Enter to install. The installer runs `go mod download` in each selected tool's directory and prints the exact config to paste into your agent.

---

## Requirements

- **Go 1.21+** - [go.dev/dl](https://go.dev/dl/)
- **Git** - [git-scm.com](https://git-scm.com/)

---

## Connecting to your agent

### Cursor / Claude Code / Windsurf (MCP stdio)

Add to your MCP config:

```json
{
  "mcpServers": {
    "claw-gmail": {
      "command": "bash",
      "args": ["-c", "cd /path/to/claw-tools.dev/tools/gmail && go run . -- --mode mcp"]
    },
    "claw-gcal": {
      "command": "bash",
      "args": ["-c", "cd /path/to/claw-tools.dev/tools/gcal && go run . -- --mode mcp"]
    }
  }
}
```

| Editor | Config location |
|--------|----------------|
| Cursor | Settings → MCP, or `~/.cursor/mcp.json` |
| Claude Code | `~/.claude/mcp.json` |
| Windsurf | Settings → MCP Servers |

### PicoClaw / OpenClaw / custom HTTP agents

```bash
cd tools/gmail && go run . --mode http --port 3101
```

Then set `CLAW_GMAIL_URL=http://localhost:3101` in your agent config.

---

## Authentication

For Google tools, run once after installing:

```bash
cd tools/gmail && go run . --auth
# or
cd tools/gcal && go run . --auth
```

Both tools share the same token at `~/.picoclaw/tokens/google.json`. You only need to auth once per Google account.

**Setup steps:**
1. Create a project at [console.cloud.google.com](https://console.cloud.google.com/)
2. Enable Gmail API and/or Google Calendar API
3. Create OAuth 2.0 credentials (Desktop app)
4. Download `credentials.json` → save as `~/.picoclaw/config/google_credentials.json`
5. Run `go run . --auth`

---

## Available tools

| ID | Name | Status | MCP functions | HTTP port |
|----|------|--------|---------------|-----------|
| `gmail` | Gmail Connector | Beta | `gmail_list`, `gmail_search`, `gmail_get` | 3101 |
| `gcal` | Google Calendar | Beta | `gcal_today`, `gcal_upcoming`, `gcal_get` | 3102 |
| `outlook` | Outlook / Exchange | Coming soon | `outlook_list`, `outlook_calendar` | 3103 |
| `weather` | Weather | Coming soon | `weather_now`, `weather_forecast` | 3104 |
| `deals` | Grocery Deals | Coming soon | `deals_search`, `deals_nearby` | 3105 |

---

## Add a new tool

1. Copy `tools/_template/` to `tools/<your-id>/`
2. Rename the module in `go.mod`
3. Implement `runMCP()` and `runHTTP()` in `main.go`
4. Add an entry to `tools.json` at repo root
5. Submit a PR

---

## Related

- [Setup Wizard](https://github.com/arpit0515/claw-setup-wizard) - lightweight personal AI agent  
- [clawtools.dev](https://claw-tools.dev) - project website

---

## License

MIT © [arpit0515](https://github.com/arpit0515)

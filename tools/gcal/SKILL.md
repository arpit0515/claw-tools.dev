# Google Calendar Tool

Read Google Calendar events for your AI agent. Powers daily schedule briefings, meeting prep, and time-aware planning.

## Install

- **Runtime**: Go 1.21+
- **Build**: `go build -o gcal-mcp .`
- **Binary**: `gcal-mcp`
- **Verify**: `./gcal-mcp --version`

## MCP

- **Mode**: stdio
- **Command**: `./gcal-mcp`
- **Args**: `--mode mcp`

## Agent Config
```json
{
  "tools": {
    "mcp": {
      "servers": {
        "claw-gcal": {
          "command": "~/.picoclaw/tools/gcal/gcal-mcp",
          "args": ["--mode", "mcp"],
          "cwd": "~/.picoclaw/tools/gcal"
        }
      }
    }
  }
}
```

## Available Tools

| Tool | Description | Parameters |
|---|---|---|
| gcal_today | Today's events | `account` |
| gcal_upcoming | Upcoming events | `days` (default 7), `account` |
| gcal_get | Get event by ID | `id` (required), `account` |
| gcal_accounts | List all connected accounts | — |

## Auth

- **Provider**: Google OAuth 2.0
- **Scopes**: `calendar.readonly`, `userinfo.email`
- **Tokens**: `~/.picoclaw/tokens/<email>.enc` (AES-256 encrypted)
- **Multi-account**: Yes — omit `account` param to query all accounts

## Example Agent Queries
```
"What's on my calendar today?"
"Any meetings in the next 3 days?"
"What time is my next event?"
```
# Gmail Tool

Read and search Gmail messages for your AI agent. Powers email summaries, morning briefings, and inbox triage.

## Install

- **Runtime**: Go 1.21+
- **Build**: `go build -o gmail-mcp .`
- **Binary**: `gmail-mcp`
- **Verify**: `./gmail-mcp --version`

## MCP

- **Mode**: stdio
- **Command**: `./gmail-mcp`
- **Args**: `--mode mcp`

## Agent Config
```json
{
  "tools": {
    "mcp": {
      "servers": {
        "claw-gmail": {
          "command": "~/.picoclaw/tools/gmail/gmail-mcp",
          "args": ["--mode", "mcp"],
          "cwd": "~/.picoclaw/tools/gmail"
        }
      }
    }
  }
}
```

## Available Tools

| Tool | Description | Parameters |
|---|---|---|
| gmail_list | List recent messages | `account`, `q`, `max` |
| gmail_search | Search by query | `q` (required), `account`, `max` |
| gmail_get | Get message by ID | `id` (required), `account` |
| gmail_accounts | List all connected accounts | — |

## Auth

- **Provider**: Google OAuth 2.0
- **Scopes**: `gmail.readonly`, `userinfo.email`
- **Tokens**: `~/.picoclaw/tokens/<email>.enc` (AES-256 encrypted)
- **Multi-account**: Yes — omit `account` param to query all accounts

## Example Agent Queries
```
"Show me my last 5 emails"
"Any unread emails from my boss?"
"Search for emails about the project proposal"
```

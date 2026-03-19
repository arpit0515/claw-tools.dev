# Gmail Tool

Read and search Gmail messages for your AI agent. Powers email summaries, morning briefings, and inbox triage.

## Service

- **URL**: http://localhost:3101
- **Health**: GET http://localhost:3101/health
- **Start**: `cd ~/claw-tools.dev/tools/gmail && go run . --mode http --port 3101`
- **Mode**: HTTP REST

## Available Tools

| Tool | Method | Endpoint | Description |
|---|---|---|---|
| gmail_list | GET | /gmail/list | List recent messages. Params: `account`, `q`, `max` |
| gmail_search | GET | /gmail/search | Search by query. Params: `q` (required), `account`, `max` |
| gmail_get | GET | /gmail/get | Get message by ID. Params: `id` (required), `account` |
| gmail_accounts | GET | /gmail/accounts | List all connected accounts |

## Auth

- **Provider**: Google OAuth 2.0
- **Scopes**: `gmail.readonly`, `userinfo.email`
- **Tokens**: `~/.picoclaw/tokens/<email>.enc` (AES-256 encrypted)
- **Multi-account**: Yes — omit `account` param to query all accounts

## Agent Config

When this tool is connected, add to `~/.picoclaw/config.json`:

```json
{
  "tools": {
    "gmail": {
      "url": "http://localhost:3101",
      "autostart": true,
      "health": "http://localhost:3101/health"
    }
  }
}
```

## Example Queries

```
GET /gmail/list?max=10
GET /gmail/search?q=from:boss@company.com+is:unread
GET /gmail/list?account=work@company.com&q=is:unread
```

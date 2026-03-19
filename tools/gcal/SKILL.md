# Google Calendar Tool

Read Google Calendar events for your AI agent. Powers daily schedule briefings, meeting prep, and time-aware planning.

## Service

- **URL**: http://localhost:3102
- **Health**: GET http://localhost:3102/health
- **Start**: `cd ~/claw-tools.dev/tools/gcal && go run . --mode http --port 3102`
- **Mode**: HTTP REST

## Available Tools

| Tool | Method | Endpoint | Description |
|---|---|---|---|
| gcal_today | GET | /gcal/today | Today's events. Params: `account` |
| gcal_upcoming | GET | /gcal/upcoming | Upcoming events. Params: `days` (default 7), `account` |
| gcal_get | GET | /gcal/get | Get event by ID. Params: `id` (required), `account` |
| gcal_accounts | GET | /gcal/accounts | List all connected accounts |

## Auth

- **Provider**: Google OAuth 2.0
- **Scopes**: `calendar.readonly`, `userinfo.email`
- **Tokens**: `~/.picoclaw/tokens/<email>.enc` (AES-256 encrypted)
- **Multi-account**: Yes — omit `account` param to query all accounts

## Agent Config

When this tool is connected, add to `~/.picoclaw/config.json`:

```json
{
  "tools": {
    "gcal": {
      "url": "http://localhost:3102",
      "autostart": true,
      "health": "http://localhost:3102/health"
    }
  }
}
```

## Example Queries

```
GET /gcal/today
GET /gcal/upcoming?days=3
GET /gcal/today?account=work@company.com
```

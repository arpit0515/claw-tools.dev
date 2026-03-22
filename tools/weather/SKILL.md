# Weather Tool

Full day weather forecasts and current conditions for your AI agent. Powers morning briefings with morning / afternoon / evening summaries and rain chance. No API key required — uses Open-Meteo.

## Install

- **Runtime**: Go 1.21+
- **Build**: `go build -o weather-mcp .`
- **Binary**: `weather-mcp`
- **Verify**: `./weather-mcp --location`

## Setup

Set your default location once (city name or lat/lon):

```bash
./weather-mcp --setlocation "Mississauga"
# ✓ Location set: Mississauga, Ontario, Canada (43.5890, -79.6441)
```

Location is stored in `~/.picoclaw/config/weather.json` and reused until changed.

## MCP

- **Mode**: stdio
- **Command**: `./weather-mcp`
- **Args**: `--mode mcp`

## Agent Config

```json
{
  "tools": {
    "mcp": {
      "servers": {
        "claw-weather": {
          "command": "~/.picoclaw/tools/weather/weather-mcp",
          "args": ["--mode", "mcp"],
          "cwd": "~/.picoclaw/tools/weather"
        }
      }
    }
  }
}
```

## Available Tools

| Tool | Description | Parameters |
|---|---|---|
| `weather_now` | Current conditions | `lat`, `lon` (optional — uses stored location) |
| `weather_forecast` | Full day forecast (morning / afternoon / evening) | `lat`, `lon` (optional — uses stored location) |

## HTTP Endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/health` | Health check |
| `GET` | `/weather/location` | Show stored location |
| `POST` | `/weather/location` | Set location (`{"city":"Mississauga"}` or `{"lat":43.5,"lon":-79.6}`) |
| `GET` | `/weather/now` | Current conditions |
| `GET` | `/weather/forecast` | Full day forecast |

## Example Output

```
🌤 Weather for Mississauga, Ontario, Canada — Monday Mar 22, 2026

🌅 Morning    8°C → 13°C   Partly cloudy  💧 10%
☀️  Afternoon  14°C → 18°C  Sunny          💧 5%
🌧  Evening   11°C → 13°C  Light rain     💧 70%

💨 Wind: 18 km/h  |  💧 Humidity: 62%
```

## Location Override

Pass `lat`/`lon` directly to use a one-off location without changing the stored default:

```
weather_forecast(lat=43.70, lon=-79.42)   # downtown Toronto
```

## Notes

- **No API key** — powered by [Open-Meteo](https://open-meteo.com/) (free, open-source)
- **Celsius** — all temperatures in °C
- **WMO weather codes** mapped to human-readable labels + emojis
- Morning: 6am–12pm | Afternoon: 12pm–6pm | Evening: 6pm–11pm
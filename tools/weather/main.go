package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// ── Config ────────────────────────────────────────────────────────────────────

type LocationConfig struct {
	Lat   float64 `json:"lat"`
	Lon   float64 `json:"lon"`
	Label string  `json:"label"`
}

func configPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".picoclaw", "config", "weather.json")
}

func loadLocation() (*LocationConfig, error) {
	data, err := os.ReadFile(configPath())
	if err != nil {
		return nil, fmt.Errorf("no location set — use --setlocation or POST /location")
	}
	var cfg LocationConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid location config: %w", err)
	}
	return &cfg, nil
}

func saveLocation(cfg LocationConfig) error {
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	return os.WriteFile(path, data, 0600)
}

// ── Geocoding (Open-Meteo) ────────────────────────────────────────────────────

type GeoResult struct {
	Name      string  `json:"name"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Country   string  `json:"country"`
	Admin1    string  `json:"admin1"` // state / province
}

type GeoResponse struct {
	Results []GeoResult `json:"results"`
}

func geocode(city string) (*LocationConfig, error) {
	url := fmt.Sprintf("https://geocoding-api.open-meteo.com/v1/search?name=%s&count=1&language=en&format=json", city)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("geocoding request failed: %w", err)
	}
	defer resp.Body.Close()

	var geo GeoResponse
	if err := json.NewDecoder(resp.Body).Decode(&geo); err != nil {
		return nil, fmt.Errorf("geocoding parse failed: %w", err)
	}
	if len(geo.Results) == 0 {
		return nil, fmt.Errorf("location not found: %s", city)
	}
	r := geo.Results[0]
	label := r.Name
	if r.Admin1 != "" {
		label += ", " + r.Admin1
	}
	if r.Country != "" {
		label += ", " + r.Country
	}
	return &LocationConfig{Lat: r.Latitude, Lon: r.Longitude, Label: label}, nil
}

// ── Weather types ─────────────────────────────────────────────────────────────

type HourlyData struct {
	Time             []string  `json:"time"`
	Temperature2m    []float64 `json:"temperature_2m"`
	ApparentTemp     []float64 `json:"apparent_temperature"`
	PrecipProb       []int     `json:"precipitation_probability"`
	WeatherCode      []int     `json:"weathercode"`
	WindSpeed        []float64 `json:"windspeed_10m"`
	RelativeHumidity []int     `json:"relativehumidity_2m"`
}

type OpenMeteoResp struct {
	Hourly         HourlyData        `json:"hourly"`
	HourlyUnits    map[string]string `json:"hourly_units"`
	CurrentWeather struct {
		Temperature float64 `json:"temperature"`
		Windspeed   float64 `json:"windspeed"`
		WeatherCode int     `json:"weathercode"`
	} `json:"current_weather"`
}

type WeatherBucket struct {
	Label      string  `json:"label"` // "Morning", "Afternoon", "Evening"
	TempMin    float64 `json:"temp_min"`
	TempMax    float64 `json:"temp_max"`
	FeelsLike  float64 `json:"feels_like"`
	Condition  string  `json:"condition"`
	RainChance int     `json:"rain_chance"`
	Emoji      string  `json:"emoji"`
}

type WeatherForecast struct {
	Location  string        `json:"location"`
	Date      string        `json:"date"`
	Morning   WeatherBucket `json:"morning"`
	Afternoon WeatherBucket `json:"afternoon"`
	Evening   WeatherBucket `json:"evening"`
	WindKmh   float64       `json:"wind_kmh"`
	Humidity  int           `json:"humidity"`
	Summary   string        `json:"summary"`
}

type WeatherNow struct {
	Location    string  `json:"location"`
	Temperature float64 `json:"temperature"`
	FeelsLike   float64 `json:"feels_like"`
	Condition   string  `json:"condition"`
	Emoji       string  `json:"emoji"`
	WindKmh     float64 `json:"wind_kmh"`
	Humidity    int     `json:"humidity"`
}

// ── WMO weather code → label + emoji ─────────────────────────────────────────

func wmoLabel(code int) (string, string) {
	switch {
	case code == 0:
		return "Clear sky", "☀️"
	case code == 1:
		return "Mainly clear", "🌤"
	case code == 2:
		return "Partly cloudy", "⛅"
	case code == 3:
		return "Overcast", "☁️"
	case code >= 45 && code <= 48:
		return "Foggy", "🌫"
	case code >= 51 && code <= 55:
		return "Drizzle", "🌦"
	case code >= 61 && code <= 65:
		return "Rain", "🌧"
	case code >= 71 && code <= 77:
		return "Snow", "❄️"
	case code >= 80 && code <= 82:
		return "Rain showers", "🌧"
	case code >= 85 && code <= 86:
		return "Snow showers", "🌨"
	case code >= 95 && code <= 99:
		return "Thunderstorm", "⛈"
	default:
		return "Unknown", "🌡"
	}
}

// ── Fetch from Open-Meteo ─────────────────────────────────────────────────────

func fetchWeather(lat, lon float64) (*OpenMeteoResp, error) {
	url := fmt.Sprintf(
		"https://api.open-meteo.com/v1/forecast?latitude=%.4f&longitude=%.4f"+
			"&hourly=temperature_2m,apparent_temperature,precipitation_probability,weathercode,windspeed_10m,relativehumidity_2m"+
			"&current_weather=true&temperature_unit=celsius&windspeed_unit=kmh&forecast_days=1",
		lat, lon,
	)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("weather fetch failed: %w", err)
	}
	defer resp.Body.Close()

	var result OpenMeteoResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("weather parse failed: %w", err)
	}
	return &result, nil
}

// ── Parse into buckets ────────────────────────────────────────────────────────

// bucketRange returns indices for hour range (inclusive)
func bucketHours(data HourlyData, startH, endH int) (temps, feels []float64, rain []int, codes []int, winds []float64, humidity []int) {
	for i, t := range data.Time {
		parsed, err := time.Parse("2006-01-02T15:04", t)
		if err != nil {
			continue
		}
		h := parsed.Hour()
		if h >= startH && h < endH {
			if i < len(data.Temperature2m) {
				temps = append(temps, data.Temperature2m[i])
			}
			if i < len(data.ApparentTemp) {
				feels = append(feels, data.ApparentTemp[i])
			}
			if i < len(data.PrecipProb) {
				rain = append(rain, data.PrecipProb[i])
			}
			if i < len(data.WeatherCode) {
				codes = append(codes, data.WeatherCode[i])
			}
			if i < len(data.WindSpeed) {
				winds = append(winds, data.WindSpeed[i])
			}
			if i < len(data.RelativeHumidity) {
				humidity = append(humidity, data.RelativeHumidity[i])
			}
		}
	}
	return
}

func minMaxFloat(vals []float64) (float64, float64) {
	if len(vals) == 0 {
		return 0, 0
	}
	mn, mx := vals[0], vals[0]
	for _, v := range vals[1:] {
		if v < mn {
			mn = v
		}
		if v > mx {
			mx = v
		}
	}
	return mn, mx
}

func avgFloat(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func maxInt(vals []int) int {
	if len(vals) == 0 {
		return 0
	}
	mx := vals[0]
	for _, v := range vals[1:] {
		if v > mx {
			mx = v
		}
	}
	return mx
}

func dominantCode(codes []int) int {
	if len(codes) == 0 {
		return 0
	}
	freq := map[int]int{}
	for _, c := range codes {
		freq[c]++
	}
	best, bestCount := 0, 0
	for c, n := range freq {
		if n > bestCount {
			best, bestCount = c, n
		}
	}
	return best
}

func makeBucket(label string, data HourlyData, startH, endH int) WeatherBucket {
	temps, feels, rain, codes, _, _ := bucketHours(data, startH, endH)
	mn, mx := minMaxFloat(temps)
	fl := avgFloat(feels)
	rc := maxInt(rain)
	code := dominantCode(codes)
	condition, emoji := wmoLabel(code)
	return WeatherBucket{
		Label:      label,
		TempMin:    round1(mn),
		TempMax:    round1(mx),
		FeelsLike:  round1(fl),
		Condition:  condition,
		RainChance: rc,
		Emoji:      emoji,
	}
}

func round1(f float64) float64 {
	return float64(int(f*10+0.5)) / 10
}

// ── Build forecast + now ──────────────────────────────────────────────────────

func buildForecast(loc *LocationConfig, raw *OpenMeteoResp) WeatherForecast {
	morning := makeBucket("Morning", raw.Hourly, 6, 12)
	afternoon := makeBucket("Afternoon", raw.Hourly, 12, 18)
	evening := makeBucket("Evening", raw.Hourly, 18, 23)

	// Overall wind + humidity from full day
	_, _, _, _, winds, humidity := bucketHours(raw.Hourly, 0, 24)
	avgWind := round1(avgFloat(winds))
	avgHum := 0
	if len(humidity) > 0 {
		sum := 0
		for _, h := range humidity {
			sum += h
		}
		avgHum = sum / len(humidity)
	}

	date := time.Now().Format("Monday Jan 2, 2006")
	summary := formatSummary(loc.Label, date, morning, afternoon, evening, avgWind, avgHum)

	return WeatherForecast{
		Location:  loc.Label,
		Date:      date,
		Morning:   morning,
		Afternoon: afternoon,
		Evening:   evening,
		WindKmh:   avgWind,
		Humidity:  avgHum,
		Summary:   summary,
	}
}

func buildNow(loc *LocationConfig, raw *OpenMeteoResp) WeatherNow {
	cw := raw.CurrentWeather
	condition, emoji := wmoLabel(cw.WeatherCode)

	// feels like: grab current hour's apparent temp
	feelsLike := cw.Temperature
	now := time.Now().Hour()
	for i, t := range raw.Hourly.Time {
		parsed, err := time.Parse("2006-01-02T15:04", t)
		if err != nil {
			continue
		}
		if parsed.Hour() == now && i < len(raw.Hourly.ApparentTemp) {
			feelsLike = raw.Hourly.ApparentTemp[i]
			break
		}
	}

	// humidity at current hour
	humidity := 0
	for i, t := range raw.Hourly.Time {
		parsed, err := time.Parse("2006-01-02T15:04", t)
		if err != nil {
			continue
		}
		if parsed.Hour() == now && i < len(raw.Hourly.RelativeHumidity) {
			humidity = raw.Hourly.RelativeHumidity[i]
			break
		}
	}

	return WeatherNow{
		Location:    loc.Label,
		Temperature: round1(cw.Temperature),
		FeelsLike:   round1(feelsLike),
		Condition:   condition,
		Emoji:       emoji,
		WindKmh:     round1(cw.Windspeed),
		Humidity:    humidity,
	}
}

// ── Format Telegram summary ───────────────────────────────────────────────────

func formatSummary(location, date string, m, a, e WeatherBucket, wind float64, humidity int) string {
	return fmt.Sprintf(
		"🌤 *Weather for %s — %s*\n\n"+
			"%s *Morning*   %.0f°C → %.0f°C  %s  💧 %d%%\n"+
			"%s *Afternoon*  %.0f°C → %.0f°C  %s  💧 %d%%\n"+
			"%s *Evening*   %.0f°C → %.0f°C  %s  💧 %d%%\n\n"+
			"💨 Wind: %.0f km/h  |  💧 Humidity: %d%%",
		location, date,
		m.Emoji, m.TempMin, m.TempMax, m.Condition, m.RainChance,
		a.Emoji, a.TempMin, a.TempMax, a.Condition, a.RainChance,
		e.Emoji, e.TempMin, e.TempMax, e.Condition, e.RainChance,
		wind, humidity,
	)
}

// ── MCP ───────────────────────────────────────────────────────────────────────

type mcpReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type mcpResp struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *mcpError   `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func writeResp(v mcpResp) {
	data, _ := json.Marshal(v)
	fmt.Println(string(data))
}

func okResp(id, result interface{}) mcpResp {
	return mcpResp{JSONRPC: "2.0", ID: id, Result: result}
}

func errResp(id interface{}, msg string) mcpResp {
	return mcpResp{JSONRPC: "2.0", ID: id, Error: &mcpError{Code: -32000, Message: msg}}
}

var toolDefs = map[string]interface{}{
	"tools": []map[string]interface{}{
		{
			"name":        "weather_now",
			"description": "Get current weather conditions for the stored location or a provided lat/lon.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"lat": map[string]interface{}{"type": "number", "description": "Latitude (optional — uses stored location if omitted)"},
					"lon": map[string]interface{}{"type": "number", "description": "Longitude (optional — uses stored location if omitted)"},
				},
			},
		},
		{
			"name":        "weather_forecast",
			"description": "Get full day weather forecast split into morning, afternoon, and evening buckets with rain chance.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"lat": map[string]interface{}{"type": "number", "description": "Latitude (optional — uses stored location if omitted)"},
					"lon": map[string]interface{}{"type": "number", "description": "Longitude (optional — uses stored location if omitted)"},
				},
			},
		},
	},
}

func resolveLocation(args map[string]interface{}) (*LocationConfig, error) {
	lat, hasLat := args["lat"].(float64)
	lon, hasLon := args["lon"].(float64)
	if hasLat && hasLon {
		return &LocationConfig{Lat: lat, Lon: lon, Label: fmt.Sprintf("%.4f, %.4f", lat, lon)}, nil
	}
	return loadLocation()
}

func runMCP() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		var req mcpReq
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue
		}

		switch req.Method {
		case "initialize":
			writeResp(okResp(req.ID, map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
				"serverInfo":      map[string]interface{}{"name": "claw-weather", "version": "1.0.0"},
			}))

		case "tools/list":
			writeResp(okResp(req.ID, toolDefs))

		case "tools/call":
			var p struct {
				Name      string                 `json:"name"`
				Arguments map[string]interface{} `json:"arguments"`
			}
			if err := json.Unmarshal(req.Params, &p); err != nil {
				writeResp(errResp(req.ID, "invalid params"))
				continue
			}

			loc, err := resolveLocation(p.Arguments)
			if err != nil {
				writeResp(errResp(req.ID, err.Error()))
				continue
			}

			raw, err := fetchWeather(loc.Lat, loc.Lon)
			if err != nil {
				writeResp(errResp(req.ID, err.Error()))
				continue
			}

			switch p.Name {
			case "weather_now":
				now := buildNow(loc, raw)
				data, _ := json.MarshalIndent(now, "", "  ")
				writeResp(okResp(req.ID, map[string]interface{}{
					"content": []map[string]interface{}{{"type": "text", "text": string(data)}},
				}))

			case "weather_forecast":
				forecast := buildForecast(loc, raw)
				data, _ := json.MarshalIndent(forecast, "", "  ")
				writeResp(okResp(req.ID, map[string]interface{}{
					"content": []map[string]interface{}{{"type": "text", "text": string(data)}},
				}))

			default:
				writeResp(errResp(req.ID, "unknown tool: "+p.Name))
			}
		}
	}
}

// ── HTTP ──────────────────────────────────────────────────────────────────────

func jsonOK(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func runHTTP(port int) {
	// GET /health
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, map[string]string{"status": "ok", "tool": "claw-weather"})
	})

	// GET /weather/location — show stored location
	http.HandleFunc("/weather/location", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			// POST /weather/location — set location by city name or lat/lon
			var body struct {
				City  string  `json:"city"`
				Lat   float64 `json:"lat"`
				Lon   float64 `json:"lon"`
				Label string  `json:"label"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				jsonErr(w, 400, "invalid JSON body")
				return
			}
			var cfg *LocationConfig
			var err error
			if body.City != "" {
				cfg, err = geocode(body.City)
				if err != nil {
					jsonErr(w, 400, err.Error())
					return
				}
			} else if body.Lat != 0 && body.Lon != 0 {
				label := body.Label
				if label == "" {
					label = fmt.Sprintf("%.4f, %.4f", body.Lat, body.Lon)
				}
				cfg = &LocationConfig{Lat: body.Lat, Lon: body.Lon, Label: label}
			} else {
				jsonErr(w, 400, "provide city or lat+lon")
				return
			}
			if err := saveLocation(*cfg); err != nil {
				jsonErr(w, 500, err.Error())
				return
			}
			jsonOK(w, cfg)
			return
		}
		// GET
		cfg, err := loadLocation()
		if err != nil {
			jsonErr(w, 404, err.Error())
			return
		}
		jsonOK(w, cfg)
	})

	// GET /weather/now?lat=...&lon=...
	http.HandleFunc("/weather/now", func(w http.ResponseWriter, r *http.Request) {
		loc, err := locationFromRequest(r)
		if err != nil {
			jsonErr(w, 400, err.Error())
			return
		}
		raw, err := fetchWeather(loc.Lat, loc.Lon)
		if err != nil {
			jsonErr(w, 500, err.Error())
			return
		}
		jsonOK(w, buildNow(loc, raw))
	})

	// GET /weather/forecast?lat=...&lon=...
	http.HandleFunc("/weather/forecast", func(w http.ResponseWriter, r *http.Request) {
		loc, err := locationFromRequest(r)
		if err != nil {
			jsonErr(w, 400, err.Error())
			return
		}
		raw, err := fetchWeather(loc.Lat, loc.Lon)
		if err != nil {
			jsonErr(w, 500, err.Error())
			return
		}
		jsonOK(w, buildForecast(loc, raw))
	})

	addr := fmt.Sprintf(":%d", port)
	log.Printf("claw-weather listening on %s (HTTP mode)", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func locationFromRequest(r *http.Request) (*LocationConfig, error) {
	q := r.URL.Query()
	latStr := q.Get("lat")
	lonStr := q.Get("lon")
	if latStr != "" && lonStr != "" {
		var lat, lon float64
		fmt.Sscanf(latStr, "%f", &lat)
		fmt.Sscanf(lonStr, "%f", &lon)
		return &LocationConfig{Lat: lat, Lon: lon, Label: fmt.Sprintf("%.4f, %.4f", lat, lon)}, nil
	}
	return loadLocation()
}

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	mode := flag.String("mode", "", "mcp | http")
	port := flag.Int("port", 3104, "HTTP port")
	setLocation := flag.String("setlocation", "", "Set location by city name (e.g. 'Mississauga')")
	showLocation := flag.Bool("location", false, "Show stored location")
	flag.Parse()

	switch {
	case *setLocation != "":
		cfg, err := geocode(*setLocation)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		if err := saveLocation(*cfg); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Location set: %s (%.4f, %.4f)\n", cfg.Label, cfg.Lat, cfg.Lon)

	case *showLocation:
		cfg, err := loadLocation()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		fmt.Printf("📍 %s (%.4f, %.4f)\n", cfg.Label, cfg.Lat, cfg.Lon)

	case *mode == "mcp":
		runMCP()

	case *mode == "http":
		runHTTP(*port)

	default:
		fmt.Fprintln(os.Stderr, `Usage:
  go run . --setlocation "Mississauga"   Set default location by city name
  go run . --location                    Show stored location
  go run . --mode mcp                    Start MCP server (stdio)
  go run . --mode http [--port 3104]     Start HTTP server`)
		os.Exit(1)
	}
}

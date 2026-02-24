package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

// ── Paths ─────────────────────────────────────────────────────────────────────

func tokenPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claw", "tokens", "google.json")
}

func credsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claw", "config", "google_credentials.json")
}

// ── OAuth2 ────────────────────────────────────────────────────────────────────

func oauthConfig() (*oauth2.Config, error) {
	data, err := os.ReadFile(credsPath())
	if err != nil {
		return nil, fmt.Errorf(
			"credentials not found at %s\n  Download from: https://console.cloud.google.com/ → APIs & Services → Credentials",
			credsPath(),
		)
	}
	cfg, err := google.ConfigFromJSON(data, calendar.CalendarReadonlyScope)
	if err != nil {
		return nil, fmt.Errorf("invalid credentials file: %w", err)
	}
	return cfg, nil
}

func loadToken() (*oauth2.Token, error) {
	data, err := os.ReadFile(tokenPath())
	if err != nil {
		return nil, err
	}
	var tok oauth2.Token
	return &tok, json.Unmarshal(data, &tok)
}

func saveToken(tok *oauth2.Token) error {
	os.MkdirAll(filepath.Dir(tokenPath()), 0700)
	data, _ := json.MarshalIndent(tok, "", "  ")
	return os.WriteFile(tokenPath(), data, 0600)
}

func newCalService(ctx context.Context) (*calendar.Service, error) {
	cfg, err := oauthConfig()
	if err != nil {
		return nil, err
	}
	tok, err := loadToken()
	if err != nil {
		return nil, fmt.Errorf("not authenticated - run: go run . --auth")
	}
	client := cfg.Client(ctx, tok)
	return calendar.NewService(ctx, option.WithHTTPClient(client))
}

func runAuth() error {
	cfg, err := oauthConfig()
	if err != nil {
		return err
	}
	url := cfg.AuthCodeURL("state", oauth2.AccessTypeOffline)
	fmt.Println("\nOpen this URL in your browser:\n\n" + url + "\n")
	fmt.Print("Paste the authorization code here: ")
	var code string
	fmt.Scan(&code)
	tok, err := cfg.Exchange(context.Background(), strings.TrimSpace(code))
	if err != nil {
		return fmt.Errorf("token exchange failed: %w", err)
	}
	if err := saveToken(tok); err != nil {
		return err
	}
	fmt.Println("\n✓ Authenticated. Token saved to", tokenPath())
	return nil
}

// ── Data ──────────────────────────────────────────────────────────────────────

type Event struct {
	ID       string `json:"id"`
	Summary  string `json:"summary"`
	Start    string `json:"start"`
	End      string `json:"end"`
	Location string `json:"location,omitempty"`
	Link     string `json:"link,omitempty"`
}

func toEvent(e *calendar.Event) Event {
	start, end := "", ""
	if e.Start != nil {
		start = e.Start.DateTime
		if start == "" {
			start = e.Start.Date
		}
	}
	if e.End != nil {
		end = e.End.DateTime
		if end == "" {
			end = e.End.Date
		}
	}
	return Event{
		ID:       e.Id,
		Summary:  e.Summary,
		Start:    start,
		End:      end,
		Location: e.Location,
		Link:     e.HtmlLink,
	}
}

func fetchToday(ctx context.Context) ([]Event, error) {
	svc, err := newCalService(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	dayEnd := dayStart.Add(24 * time.Hour)

	resp, err := svc.Events.List("primary").
		TimeMin(dayStart.Format(time.RFC3339)).
		TimeMax(dayEnd.Format(time.RFC3339)).
		SingleEvents(true).
		OrderBy("startTime").
		Do()
	if err != nil {
		return nil, err
	}
	events := make([]Event, 0, len(resp.Items))
	for _, e := range resp.Items {
		events = append(events, toEvent(e))
	}
	return events, nil
}

func fetchUpcoming(ctx context.Context, days int) ([]Event, error) {
	svc, err := newCalService(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	until := now.Add(time.Duration(days) * 24 * time.Hour)

	resp, err := svc.Events.List("primary").
		TimeMin(now.Format(time.RFC3339)).
		TimeMax(until.Format(time.RFC3339)).
		SingleEvents(true).
		OrderBy("startTime").
		MaxResults(50).
		Do()
	if err != nil {
		return nil, err
	}
	events := make([]Event, 0, len(resp.Items))
	for _, e := range resp.Items {
		events = append(events, toEvent(e))
	}
	return events, nil
}

func fetchEvent(ctx context.Context, id string) (*Event, error) {
	svc, err := newCalService(ctx)
	if err != nil {
		return nil, err
	}
	e, err := svc.Events.Get("primary", id).Do()
	if err != nil {
		return nil, err
	}
	ev := toEvent(e)
	return &ev, nil
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

func okResp(id interface{}, result interface{}) mcpResp {
	return mcpResp{JSONRPC: "2.0", ID: id, Result: result}
}

func errResp(id interface{}, msg string) mcpResp {
	return mcpResp{JSONRPC: "2.0", ID: id, Error: &mcpError{Code: -32000, Message: msg}}
}

var toolDefs = map[string]interface{}{
	"tools": []map[string]interface{}{
		{
			"name":        "gcal_today",
			"description": "Get all events on today's Google Calendar",
			"inputSchema": map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
		{
			"name":        "gcal_upcoming",
			"description": "Get upcoming Google Calendar events",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"days": map[string]interface{}{"type": "integer", "description": "Number of days ahead to look (default 7)"},
				},
			},
		},
		{
			"name":        "gcal_get",
			"description": "Get a Google Calendar event by ID",
			"inputSchema": map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{"id": map[string]interface{}{"type": "string"}},
				"required":   []string{"id"},
			},
		},
	},
}

func runMCP() {
	ctx := context.Background()
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
				"serverInfo":      map[string]interface{}{"name": "claw-gcal", "version": "0.1.0"},
			}))

		case "tools/list":
			writeResp(okResp(req.ID, toolDefs))

		case "tools/call":
			var p struct {
				Name      string                 `json:"name"`
				Arguments map[string]interface{} `json:"arguments"`
			}
			json.Unmarshal(req.Params, &p)

			switch p.Name {
			case "gcal_today":
				events, err := fetchToday(ctx)
				if err != nil {
					writeResp(errResp(req.ID, err.Error()))
					continue
				}
				data, _ := json.MarshalIndent(events, "", "  ")
				writeResp(okResp(req.ID, map[string]interface{}{
					"content": []map[string]interface{}{{"type": "text", "text": string(data)}},
				}))

			case "gcal_upcoming":
				days := 7
				if n, ok := p.Arguments["days"].(float64); ok {
					days = int(n)
				}
				events, err := fetchUpcoming(ctx, days)
				if err != nil {
					writeResp(errResp(req.ID, err.Error()))
					continue
				}
				data, _ := json.MarshalIndent(events, "", "  ")
				writeResp(okResp(req.ID, map[string]interface{}{
					"content": []map[string]interface{}{{"type": "text", "text": string(data)}},
				}))

			case "gcal_get":
				id, _ := p.Arguments["id"].(string)
				event, err := fetchEvent(ctx, id)
				if err != nil {
					writeResp(errResp(req.ID, err.Error()))
					continue
				}
				data, _ := json.MarshalIndent(event, "", "  ")
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
	ctx := context.Background()

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, map[string]string{"status": "ok", "tool": "claw-gcal"})
	})
	http.HandleFunc("/gcal/today", func(w http.ResponseWriter, r *http.Request) {
		events, err := fetchToday(ctx)
		if err != nil {
			jsonErr(w, 500, err.Error())
			return
		}
		jsonOK(w, events)
	})
	http.HandleFunc("/gcal/upcoming", func(w http.ResponseWriter, r *http.Request) {
		days := 7
		if d := r.URL.Query().Get("days"); d != "" {
			fmt.Sscanf(d, "%d", &days)
		}
		events, err := fetchUpcoming(ctx, days)
		if err != nil {
			jsonErr(w, 500, err.Error())
			return
		}
		jsonOK(w, events)
	})
	http.HandleFunc("/gcal/get", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			jsonErr(w, 400, "id required")
			return
		}
		event, err := fetchEvent(ctx, id)
		if err != nil {
			jsonErr(w, 500, err.Error())
			return
		}
		jsonOK(w, event)
	})

	addr := fmt.Sprintf(":%d", port)
	log.Printf("claw-gcal listening on %s (HTTP mode)", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	mode := flag.String("mode", "", "mcp | http")
	port := flag.Int("port", 3102, "HTTP port")
	doAuth := flag.Bool("auth", false, "Run OAuth2 setup")
	flag.Parse()

	if *doAuth {
		if err := runAuth(); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}

	switch *mode {
	case "mcp":
		runMCP()
	case "http":
		runHTTP(*port)
	default:
		fmt.Fprintln(os.Stderr, "Usage: go run . --mode mcp|http [--port 3102] [--auth]")
		os.Exit(1)
	}
}

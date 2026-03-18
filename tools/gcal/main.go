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
	"time"

	"github.com/arpit0515/claw-tools.dev/tools/shared"
	"golang.org/x/oauth2"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

// ── GCal scopes ───────────────────────────────────────────────────────────────

var gcalScopes = []string{
	calendar.CalendarReadonlyScope,
	"https://www.googleapis.com/auth/userinfo.email",
}

// ── Service factory ───────────────────────────────────────────────────────────

func newCalService(ctx context.Context, email string) (*calendar.Service, error) {
	cfg, err := shared.NewOAuthConfig(gcalScopes...)
	if err != nil {
		return nil, err
	}
	if email == "" {
		email, err = shared.DefaultAccount()
		if err != nil {
			return nil, err
		}
	}
	client, err := shared.GetAuthenticatedClient(cfg, email)
	if err != nil {
		return nil, err
	}
	return calendar.NewService(ctx, option.WithHTTPClient(client))
}

// ── Auth ──────────────────────────────────────────────────────────────────────

func runAuth() error {
	cfg, err := shared.NewOAuthConfig(gcalScopes...)
	if err != nil {
		return err
	}
	result, err := shared.RunAuthFlow(cfg)
	if err != nil {
		return err
	}
	fmt.Printf("\n✓ Connected: %s\n", result.Email)
	fmt.Printf("  Token saved to: %s\n\n", shared.TokenPathForAccount(result.Email))
	return nil
}

// ── Data types ────────────────────────────────────────────────────────────────

type Event struct {
    ID          string   `json:"id"`
    Summary     string   `json:"summary"`
    Start       string   `json:"start"`
    End         string   `json:"end"`
    Location    string   `json:"location,omitempty"`
    Link        string   `json:"link,omitempty"`
    MeetLink    string   `json:"meet_link,omitempty"`
    Description string   `json:"description,omitempty"`
    Attendees   []string `json:"attendees,omitempty"`
    Account     string   `json:"account,omitempty"`
}

func toEvent(e *calendar.Event, account string) Event {
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
	ev := Event{
        ID:          e.Id,
        Summary:     e.Summary,
        Start:       start,
        End:         end,
        Location:    e.Location,
        Link:        e.HtmlLink,
        Description: e.Description,
        Account:     account,
    }

    // Google Meet link
    if e.ConferenceData != nil {
        for _, ep := range e.ConferenceData.EntryPoints {
            if ep.EntryPointType == "video" {
                ev.MeetLink = ep.Uri
                break
            }
        }
    }

    // Attendees
    for _, a := range e.Attendees {
        ev.Attendees = append(ev.Attendees, a.Email)
    }

    return ev
}

// ── Data fetchers ─────────────────────────────────────────────────────────────

func fetchToday(ctx context.Context, email string) ([]Event, error) {
	svc, err := newCalService(ctx, email)
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
		events = append(events, toEvent(e, email))
	}
	return events, nil
}

func fetchUpcoming(ctx context.Context, email string, days int) ([]Event, error) {
	svc, err := newCalService(ctx, email)
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
		events = append(events, toEvent(e, email))
	}
	return events, nil
}

func fetchEvent(ctx context.Context, email, id string) (*Event, error) {
	svc, err := newCalService(ctx, email)
	if err != nil {
		return nil, err
	}
	e, err := svc.Events.Get("primary", id).Do()
	if err != nil {
		return nil, err
	}
	ev := toEvent(e, email)
	return &ev, nil
}

// fetchTodayAllAccounts fetches today's events across all connected accounts
func fetchTodayAllAccounts(ctx context.Context) ([]Event, error) {
	accounts, err := shared.ListAccounts()
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, fmt.Errorf("no accounts connected — run: go run . --auth")
	}
	all := []Event{}
	for _, acc := range accounts {
		events, err := fetchToday(ctx, acc.Email)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", acc.Email, err)
			continue
		}
		all = append(all, events...)
	}
	return all, nil
}

// fetchUpcomingAllAccounts fetches upcoming events across all connected accounts
func fetchUpcomingAllAccounts(ctx context.Context, days int) ([]Event, error) {
	accounts, err := shared.ListAccounts()
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, fmt.Errorf("no accounts connected — run: go run . --auth")
	}
	all := []Event{}
	for _, acc := range accounts {
		events, err := fetchUpcoming(ctx, acc.Email, days)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", acc.Email, err)
			continue
		}
		all = append(all, events...)
	}
	return all, nil
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
			"name":        "gcal_today",
			"description": "Get all events on today's Google Calendar across one or all connected accounts.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"account": map[string]interface{}{"type": "string", "description": "Email address of account to use. Omit for all accounts."},
				},
			},
		},
		{
			"name":        "gcal_upcoming",
			"description": "Get upcoming Google Calendar events across one or all connected accounts.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"days":    map[string]interface{}{"type": "integer", "description": "Number of days ahead to look (default 7)"},
					"account": map[string]interface{}{"type": "string", "description": "Email address of account to use. Omit for all accounts."},
				},
			},
		},
		{
			"name":        "gcal_get",
			"description": "Get a Google Calendar event by ID.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id":      map[string]interface{}{"type": "string", "description": "Calendar event ID"},
					"account": map[string]interface{}{"type": "string", "description": "Email address of account the event belongs to"},
				},
				"required": []string{"id"},
			},
		},
		{
			"name":        "gcal_accounts",
			"description": "List all connected Google Calendar accounts.",
			"inputSchema": map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
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
				"serverInfo":      map[string]interface{}{"name": "claw-gcal", "version": "1.0.0"},
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

			account, _ := p.Arguments["account"].(string)

			switch p.Name {
			case "gcal_today":
				var events []Event
				var err error
				if account != "" {
					events, err = fetchToday(ctx, account)
				} else {
					events, err = fetchTodayAllAccounts(ctx)
				}
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
				var events []Event
				var err error
				if account != "" {
					events, err = fetchUpcoming(ctx, account, days)
				} else {
					events, err = fetchUpcomingAllAccounts(ctx, days)
				}
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
				if id == "" {
					writeResp(errResp(req.ID, "id required"))
					continue
				}
				event, err := fetchEvent(ctx, account, id)
				if err != nil {
					writeResp(errResp(req.ID, err.Error()))
					continue
				}
				data, _ := json.MarshalIndent(event, "", "  ")
				writeResp(okResp(req.ID, map[string]interface{}{
					"content": []map[string]interface{}{{"type": "text", "text": string(data)}},
				}))

			case "gcal_accounts":
				accounts, err := shared.ListAccounts()
				if err != nil {
					writeResp(errResp(req.ID, err.Error()))
					continue
				}
				data, _ := json.MarshalIndent(accounts, "", "  ")
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

	// GET /gcal/accounts
	http.HandleFunc("/gcal/accounts", func(w http.ResponseWriter, r *http.Request) {
		accounts, err := shared.ListAccounts()
		if err != nil {
			jsonErr(w, 500, err.Error())
			return
		}
		jsonOK(w, accounts)
	})

	// GET /gcal/today?account=x@gmail.com
	http.HandleFunc("/gcal/today", func(w http.ResponseWriter, r *http.Request) {
		account := r.URL.Query().Get("account")
		var events []Event
		var err error
		if account != "" {
			events, err = fetchToday(ctx, account)
		} else {
			events, err = fetchTodayAllAccounts(ctx)
		}
		if err != nil {
			jsonErr(w, 500, err.Error())
			return
		}
		jsonOK(w, events)
	})

	// GET /gcal/upcoming?days=7&account=x@gmail.com
	http.HandleFunc("/gcal/upcoming", func(w http.ResponseWriter, r *http.Request) {
		days := 7
		fmt.Sscanf(r.URL.Query().Get("days"), "%d", &days)
		account := r.URL.Query().Get("account")
		var events []Event
		var err error
		if account != "" {
			events, err = fetchUpcoming(ctx, account, days)
		} else {
			events, err = fetchUpcomingAllAccounts(ctx, days)
		}
		if err != nil {
			jsonErr(w, 500, err.Error())
			return
		}
		jsonOK(w, events)
	})

	// GET /gcal/get?id=...&account=x@gmail.com
	http.HandleFunc("/gcal/get", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			jsonErr(w, 400, "id required")
			return
		}
		account := r.URL.Query().Get("account")
		event, err := fetchEvent(ctx, account, id)
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
	mode    := flag.String("mode", "", "mcp | http")
	port    := flag.Int("port", 3102, "HTTP port")
	doAuth  := flag.Bool("auth", false, "Add a Google account via OAuth")
	listAcc := flag.Bool("accounts", false, "List connected accounts")
	revoke  := flag.String("revoke", "", "Revoke and remove an account (email address)")
	flag.Parse()

	switch {
	case *doAuth:
		if err := runAuth(); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}

	case *listAcc:
		accounts, err := shared.ListAccounts()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		if len(accounts) == 0 {
			fmt.Println("No accounts connected. Run: go run . --auth")
			return
		}
		fmt.Printf("Connected accounts (%d):\n", len(accounts))
		for _, a := range accounts {
			fmt.Printf("  • %s  (added %s)\n", a.Email, a.AddedAt.Format("2006-01-02"))
		}

	case *revoke != "":
		if err := revokeAccount(*revoke); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Account %s revoked and removed\n", *revoke)

	case *mode == "mcp":
		runMCP()

	case *mode == "http":
		runHTTP(*port)

	default:
		fmt.Fprintln(os.Stderr, `Usage:
  go run . --auth                      Add a Google account
  go run . --accounts                  List connected accounts
  go run . --revoke user@gmail.com     Remove an account
  go run . --mode mcp                  Start MCP server (stdio)
  go run . --mode http [--port 3102]   Start HTTP server`)
		os.Exit(1)
	}
}

// revokeAccount calls Google's revoke endpoint and deletes the local token
func revokeAccount(email string) error {
	tok, err := shared.LoadToken(email)
	if err != nil {
		return err
	}
	token := tok.AccessToken
	if tok.RefreshToken != "" {
		token = tok.RefreshToken
	}
	resp, err := (&oauth2.Config{}).Client(context.Background(), tok).
		Get("https://oauth2.googleapis.com/revoke?token=" + token)
	if err == nil {
		resp.Body.Close()
	}
	return shared.DeleteToken(email)
}

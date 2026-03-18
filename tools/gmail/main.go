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

	"github.com/arpit0515/claw-tools.dev/shared"
	"golang.org/x/oauth2"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// ── Gmail scopes ──────────────────────────────────────────────────────────────

var gmailScopes = []string{
	gmail.GmailReadonlyScope,
	"https://www.googleapis.com/auth/userinfo.email",
}

// ── Service factory ───────────────────────────────────────────────────────────

func newGmailService(ctx context.Context, email string) (*gmail.Service, error) {
	cfg, err := shared.NewOAuthConfig(gmailScopes...)
	if err != nil {
		return nil, err
	}
	// If no account specified, use the first connected one
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
	return gmail.NewService(ctx, option.WithHTTPClient(client))
}

// ── Auth ──────────────────────────────────────────────────────────────────────

func runAuth() error {
	cfg, err := shared.NewOAuthConfig(gmailScopes...)
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

type Message struct {
	ID      string `json:"id"`
	Subject string `json:"subject"`
	From    string `json:"from"`
	Date    string `json:"date"`
	Snippet string `json:"snippet"`
	Account string `json:"account,omitempty"`
}

// ── Data fetchers ─────────────────────────────────────────────────────────────

func fetchMessages(ctx context.Context, email string, maxResults int64, query string) ([]Message, error) {
	svc, err := newGmailService(ctx, email)
	if err != nil {
		return nil, err
	}
	call := svc.Users.Messages.List("me").MaxResults(maxResults)
	if query != "" {
		call = call.Q(query)
	}
	resp, err := call.Do()
	if err != nil {
		return nil, err
	}
	msgs := make([]Message, 0, len(resp.Messages))
	for _, m := range resp.Messages {
		full, err := svc.Users.Messages.Get("me", m.Id).
			Format("metadata").MetadataHeaders("Subject", "From", "Date").Do()
		if err != nil {
			continue
		}
		msg := Message{ID: m.Id, Snippet: full.Snippet, Account: email}
		for _, h := range full.Payload.Headers {
			switch h.Name {
			case "Subject":
				msg.Subject = h.Value
			case "From":
				msg.From = h.Value
			case "Date":
				msg.Date = h.Value
			}
		}
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

func fetchMessage(ctx context.Context, email, id string) (*Message, error) {
	svc, err := newGmailService(ctx, email)
	if err != nil {
		return nil, err
	}
	full, err := svc.Users.Messages.Get("me", id).
		Format("metadata").MetadataHeaders("Subject", "From", "Date").Do()
	if err != nil {
		return nil, err
	}
	msg := &Message{ID: id, Snippet: full.Snippet, Account: email}
	for _, h := range full.Payload.Headers {
		switch h.Name {
		case "Subject":
			msg.Subject = h.Value
		case "From":
			msg.From = h.Value
		case "Date":
			msg.Date = h.Value
		}
	}
	return msg, nil
}

// fetchAllAccounts fetches messages across all connected accounts and merges results
func fetchAllAccounts(ctx context.Context, maxResults int64, query string) ([]Message, error) {
	accounts, err := shared.ListAccounts()
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, fmt.Errorf("no accounts connected — run: go run . --auth")
	}
	all := []Message{}
	for _, acc := range accounts {
		msgs, err := fetchMessages(ctx, acc.Email, maxResults, query)
		if err != nil {
			// Log and continue — don't fail all accounts if one is broken
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", acc.Email, err)
			continue
		}
		all = append(all, msgs...)
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
			"name":        "gmail_list",
			"description": "List recent Gmail messages. Optionally filter by account.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"max_results": map[string]interface{}{"type": "integer", "description": "Max messages to return (default 10)"},
					"query":       map[string]interface{}{"type": "string", "description": "Optional Gmail search query"},
					"account":     map[string]interface{}{"type": "string", "description": "Email address of account to use. Omit to query all accounts."},
				},
			},
		},
		{
			"name":        "gmail_search",
			"description": "Search Gmail messages by query string across one or all accounts.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query":       map[string]interface{}{"type": "string", "description": "Gmail search query (e.g. 'from:boss@company.com is:unread')"},
					"max_results": map[string]interface{}{"type": "integer", "description": "Max results (default 20)"},
					"account":     map[string]interface{}{"type": "string", "description": "Email address of account to search. Omit to search all accounts."},
				},
				"required": []string{"query"},
			},
		},
		{
			"name":        "gmail_get",
			"description": "Get a single Gmail message by ID.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id":      map[string]interface{}{"type": "string", "description": "Gmail message ID"},
					"account": map[string]interface{}{"type": "string", "description": "Email address of account the message belongs to"},
				},
				"required": []string{"id"},
			},
		},
		{
			"name":        "gmail_accounts",
			"description": "List all connected Gmail accounts.",
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
				"serverInfo":      map[string]interface{}{"name": "claw-gmail", "version": "1.0.0"},
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
			case "gmail_list", "gmail_search":
				var max int64 = 10
				if p.Name == "gmail_search" {
					max = 20
				}
				if n, ok := p.Arguments["max_results"].(float64); ok {
					max = int64(n)
				}
				query, _ := p.Arguments["query"].(string)

				var msgs []Message
				var err error
				if account != "" {
					msgs, err = fetchMessages(ctx, account, max, query)
				} else {
					msgs, err = fetchAllAccounts(ctx, max, query)
				}
				if err != nil {
					writeResp(errResp(req.ID, err.Error()))
					continue
				}
				data, _ := json.MarshalIndent(msgs, "", "  ")
				writeResp(okResp(req.ID, map[string]interface{}{
					"content": []map[string]interface{}{{"type": "text", "text": string(data)}},
				}))

			case "gmail_get":
				id, _ := p.Arguments["id"].(string)
				if id == "" {
					writeResp(errResp(req.ID, "id required"))
					continue
				}
				msg, err := fetchMessage(ctx, account, id)
				if err != nil {
					writeResp(errResp(req.ID, err.Error()))
					continue
				}
				data, _ := json.MarshalIndent(msg, "", "  ")
				writeResp(okResp(req.ID, map[string]interface{}{
					"content": []map[string]interface{}{{"type": "text", "text": string(data)}},
				}))

			case "gmail_accounts":
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
		jsonOK(w, map[string]string{"status": "ok", "tool": "claw-gmail"})
	})

	// GET /gmail/accounts
	http.HandleFunc("/gmail/accounts", func(w http.ResponseWriter, r *http.Request) {
		accounts, err := shared.ListAccounts()
		if err != nil {
			jsonErr(w, 500, err.Error())
			return
		}
		jsonOK(w, accounts)
	})

	// GET /gmail/list?account=x@gmail.com&q=...&max=10
	http.HandleFunc("/gmail/list", func(w http.ResponseWriter, r *http.Request) {
		account := r.URL.Query().Get("account")
		query := r.URL.Query().Get("q")
		var max int64 = 10
		fmt.Sscanf(r.URL.Query().Get("max"), "%d", &max)

		var msgs []Message
		var err error
		if account != "" {
			msgs, err = fetchMessages(ctx, account, max, query)
		} else {
			msgs, err = fetchAllAccounts(ctx, max, query)
		}
		if err != nil {
			jsonErr(w, 500, err.Error())
			return
		}
		jsonOK(w, msgs)
	})

	// GET /gmail/search?q=...&account=x@gmail.com&max=20
	http.HandleFunc("/gmail/search", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q == "" {
			jsonErr(w, 400, "q parameter required")
			return
		}
		account := r.URL.Query().Get("account")
		var max int64 = 20
		fmt.Sscanf(r.URL.Query().Get("max"), "%d", &max)

		var msgs []Message
		var err error
		if account != "" {
			msgs, err = fetchMessages(ctx, account, max, q)
		} else {
			msgs, err = fetchAllAccounts(ctx, max, q)
		}
		if err != nil {
			jsonErr(w, 500, err.Error())
			return
		}
		jsonOK(w, msgs)
	})

	// GET /gmail/get?id=...&account=x@gmail.com
	http.HandleFunc("/gmail/get", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			jsonErr(w, 400, "id parameter required")
			return
		}
		account := r.URL.Query().Get("account")
		msg, err := fetchMessage(ctx, account, id)
		if err != nil {
			jsonErr(w, 500, err.Error())
			return
		}
		jsonOK(w, msg)
	})

	addr := fmt.Sprintf(":%d", port)
	log.Printf("claw-gmail listening on %s (HTTP mode)", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	mode    := flag.String("mode", "", "mcp | http")
	port    := flag.Int("port", 3101, "HTTP port")
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
  go run . --mode http [--port 3101]   Start HTTP server`)
		os.Exit(1)
	}
}

// revokeAccount calls Google's revoke endpoint and deletes the local token
func revokeAccount(email string) error {
	tok, err := shared.LoadToken(email)
	if err != nil {
		return err
	}
	// Best-effort revoke on Google's side
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

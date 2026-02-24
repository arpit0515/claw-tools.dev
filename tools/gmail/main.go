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

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
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
	cfg, err := google.ConfigFromJSON(data, gmail.GmailReadonlyScope)
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
	if err := os.MkdirAll(filepath.Dir(tokenPath()), 0700); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(tok, "", "  ")
	return os.WriteFile(tokenPath(), data, 0600)
}

func newGmailService(ctx context.Context) (*gmail.Service, error) {
	cfg, err := oauthConfig()
	if err != nil {
		return nil, err
	}
	tok, err := loadToken()
	if err != nil {
		return nil, fmt.Errorf("not authenticated - run: go run . --auth")
	}
	client := cfg.Client(ctx, tok)
	return gmail.NewService(ctx, option.WithHTTPClient(client))
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

// ── Data types ────────────────────────────────────────────────────────────────

type Message struct {
	ID      string `json:"id"`
	Subject string `json:"subject"`
	From    string `json:"from"`
	Date    string `json:"date"`
	Snippet string `json:"snippet"`
}

func fetchMessages(ctx context.Context, maxResults int64, query string) ([]Message, error) {
	svc, err := newGmailService(ctx)
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
		msg := Message{ID: m.Id, Snippet: full.Snippet}
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

func fetchMessage(ctx context.Context, id string) (*Message, error) {
	svc, err := newGmailService(ctx)
	if err != nil {
		return nil, err
	}
	full, err := svc.Users.Messages.Get("me", id).
		Format("metadata").MetadataHeaders("Subject", "From", "Date").Do()
	if err != nil {
		return nil, err
	}
	msg := &Message{ID: id, Snippet: full.Snippet}
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

// ── MCP (JSON-RPC over stdio) ─────────────────────────────────────────────────

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
			"name":        "gmail_list",
			"description": "List recent Gmail messages",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"max_results": map[string]interface{}{"type": "integer", "description": "Max messages to return (default 10)"},
					"query":       map[string]interface{}{"type": "string", "description": "Optional Gmail search query"},
				},
			},
		},
		{
			"name":        "gmail_search",
			"description": "Search Gmail messages by query string",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query":       map[string]interface{}{"type": "string", "description": "Gmail search query (e.g. 'from:boss@company.com')"},
					"max_results": map[string]interface{}{"type": "integer", "description": "Max results (default 20)"},
				},
				"required": []string{"query"},
			},
		},
		{
			"name":        "gmail_get",
			"description": "Get a single Gmail message by ID",
			"inputSchema": map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{"id": map[string]interface{}{"type": "string", "description": "Gmail message ID"}},
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
				"serverInfo":      map[string]interface{}{"name": "claw-gmail", "version": "0.1.0"},
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
			switch p.Name {
			case "gmail_list", "gmail_search":
				var max int64 = 10
				if n, ok := p.Arguments["max_results"].(float64); ok {
					max = int64(n)
				}
				query, _ := p.Arguments["query"].(string)
				msgs, err := fetchMessages(ctx, max, query)
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
				msg, err := fetchMessage(ctx, id)
				if err != nil {
					writeResp(errResp(req.ID, err.Error()))
					continue
				}
				data, _ := json.MarshalIndent(msg, "", "  ")
				writeResp(okResp(req.ID, map[string]interface{}{
					"content": []map[string]interface{}{{"type": "text", "text": string(data)}},
				}))

			default:
				writeResp(errResp(req.ID, "unknown tool: "+p.Name))
			}
		}
	}
}

// ── HTTP mode ─────────────────────────────────────────────────────────────────

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
	http.HandleFunc("/gmail/list", func(w http.ResponseWriter, r *http.Request) {
		msgs, err := fetchMessages(ctx, 10, r.URL.Query().Get("q"))
		if err != nil {
			jsonErr(w, 500, err.Error())
			return
		}
		jsonOK(w, msgs)
	})
	http.HandleFunc("/gmail/search", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q == "" {
			jsonErr(w, 400, "q parameter required")
			return
		}
		msgs, err := fetchMessages(ctx, 20, q)
		if err != nil {
			jsonErr(w, 500, err.Error())
			return
		}
		jsonOK(w, msgs)
	})
	http.HandleFunc("/gmail/get", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			jsonErr(w, 400, "id parameter required")
			return
		}
		msg, err := fetchMessage(ctx, id)
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
	mode := flag.String("mode", "", "mcp | http")
	port := flag.Int("port", 3101, "HTTP port (--mode http)")
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
		fmt.Fprintln(os.Stderr, "Usage: go run . --mode mcp|http [--port 3101] [--auth]")
		os.Exit(1)
	}
}

package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
)

// ─────────────────────────────────────────────────────────────────────────────
//  ClawTools - tool template
//  Copy this folder to tools/<your-tool-id>/ and implement:
//    1. Your data-fetching functions
//    2. toolDefs  - MCP tool definitions
//    3. tools/call handler in runMCP()
//    4. HTTP routes in runHTTP()
//
//  Then add an entry to tools.json at the repo root.
// ─────────────────────────────────────────────────────────────────────────────

// ── MCP helpers (copy as-is into your tool) ───────────────────────────────────

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

func writeResp(v mcpResp) { data, _ := json.Marshal(v); fmt.Println(string(data)) }
func okResp(id interface{}, result interface{}) mcpResp {
	return mcpResp{JSONRPC: "2.0", ID: id, Result: result}
}
func errResp(id interface{}, msg string) mcpResp {
	return mcpResp{JSONRPC: "2.0", ID: id, Error: &mcpError{Code: -32000, Message: msg}}
}

// ── Tool definitions ──────────────────────────────────────────────────────────
// Update these to match your tool's functions

var toolDefs = map[string]interface{}{
	"tools": []map[string]interface{}{
		{
			"name":        "mytool_hello",
			"description": "An example tool function - replace with your own",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{"type": "string", "description": "A name to greet"},
				},
				"required": []string{"name"},
			},
		},
	},
}

// ── Your tool logic ───────────────────────────────────────────────────────────
// Replace this with real API calls, file reads, etc.

func helloTool(name string) string {
	return fmt.Sprintf("Hello, %s! Replace this with your tool logic.", name)
}

// ── MCP mode ──────────────────────────────────────────────────────────────────

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
				"serverInfo":      map[string]interface{}{"name": "claw-mytool", "version": "0.1.0"},
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
			case "mytool_hello":
				name, _ := p.Arguments["name"].(string)
				result := helloTool(name)
				writeResp(okResp(req.ID, map[string]interface{}{
					"content": []map[string]interface{}{{"type": "text", "text": result}},
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
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, map[string]string{"status": "ok", "tool": "claw-mytool"})
	})

	// Add your HTTP routes here, e.g.:
	// http.HandleFunc("/mytool/hello", func(w http.ResponseWriter, r *http.Request) { ... })

	addr := fmt.Sprintf(":%d", port)
	log.Printf("claw-mytool listening on %s (HTTP mode)", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	mode := flag.String("mode", "", "mcp | http")
	port := flag.Int("port", 3199, "HTTP port")
	flag.Parse()

	switch *mode {
	case "mcp":
		runMCP()
	case "http":
		runHTTP(*port)
	default:
		fmt.Fprintln(os.Stderr, "Usage: go run . --mode mcp|http [--port 3199]")
		os.Exit(1)
	}
}

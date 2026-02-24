package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ── ANSI ──────────────────────────────────────────────────────────────────────

const (
	cReset = "\033[0m"
	cBold  = "\033[1m"
	cAmber = "\033[38;5;214m"
	cGreen = "\033[38;5;82m"
	cRed   = "\033[38;5;196m"
	cBlue  = "\033[38;5;75m"
	cFaint = "\033[38;5;240m"
	cWhite = "\033[97m"
)

func amber(s string) string { return cAmber + s + cReset }
func green(s string) string { return cGreen + s + cReset }
func red(s string) string   { return cRed + s + cReset }
func bold(s string) string  { return cBold + s + cReset }
func faint(s string) string { return cFaint + s + cReset }

// ── Types ─────────────────────────────────────────────────────────────────────

type Tool struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Dir          string   `json:"dir"` // relative to repo root, e.g. "tools/gmail"
	Status       string   `json:"status"` // "available" | "coming-soon"
	RequiresAuth []string `json:"requires_auth"`
	MCPTools     []string `json:"mcp_tools"`
	HTTPPort     int      `json:"http_port"`
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func clearScreen() {
	if runtime.GOOS == "windows" {
		exec.Command("cmd", "/c", "cls").Run()
	} else {
		fmt.Print("\033[H\033[2J")
	}
}

func printLogo() {
	fmt.Println()
	fmt.Println(cAmber + `  ╔══════════════════════════════════════════╗` + cReset)
	fmt.Println(cAmber + `  ║  🦞  ` + cBold + cWhite + `ClawTools Installer` + cReset + cAmber + `                ║` + cReset)
	fmt.Println(cAmber + `  ║  ` + cFaint + `MCP tool connectors for AI agents` + cAmber + `      ║` + cReset)
	fmt.Println(cAmber + `  ╚══════════════════════════════════════════╝` + cReset)
	fmt.Println()
}

func divider() {
	fmt.Println(faint("  ──────────────────────────────────────────────"))
}

func readLine() string {
	r := bufio.NewReader(os.Stdin)
	line, _ := r.ReadString('\n')
	return strings.TrimSpace(line)
}

func pressEnter() {
	fmt.Print(faint("  Press Enter to continue... "))
	readLine()
}

// ── Load tools.json ───────────────────────────────────────────────────────────

func loadTools(repoRoot string) ([]Tool, error) {
	path := filepath.Join(repoRoot, "tools.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read tools.json at %s: %w", path, err)
	}
	var tools []Tool
	if err := json.Unmarshal(data, &tools); err != nil {
		return nil, fmt.Errorf("invalid tools.json: %w", err)
	}
	return tools, nil
}

// ── Tool selection UI ─────────────────────────────────────────────────────────

func drawMenu(tools []Tool, available []int, selected map[int]bool) {
	clearScreen()
	printLogo()

	fmt.Println(bold("  Select tools to install"))
	fmt.Println(faint("  Type a number to toggle  ·  Enter when ready  ·  q to quit"))
	fmt.Println()

	// count coming-soon
	comingSoon := []Tool{}
	for _, t := range tools {
		if t.Status != "available" {
			comingSoon = append(comingSoon, t)
		}
	}

	fmt.Println(amber("  ● Available"))
	fmt.Println()

	for pos, idx := range available {
		t := tools[idx]
		displayNum := pos + 1

		checkbox := faint("[ ]")
		nameStr := faint(t.Name)
		if selected[idx] {
			checkbox = green("[✓]")
			nameStr = bold(t.Name)
		}

		fmt.Printf("  %s  %s%d%s  %s\n", checkbox, cAmber, displayNum, cReset, nameStr)
		fmt.Printf("         %s\n", faint(t.Description))

		// mcp tool names
		toolTags := ""
		for i, m := range t.MCPTools {
			if i > 0 {
				toolTags += "  "
			}
			toolTags += cFaint + m + cReset
		}
		fmt.Printf("         %s\n", toolTags)
		fmt.Println()
	}

	if len(comingSoon) > 0 {
		fmt.Println(faint("  ○ Coming soon"))
		fmt.Println()
		for i, t := range comingSoon {
			fmt.Printf("  %s  %s%d%s  %s\n",
				faint("[ ]"),
				cFaint, len(available)+i+1, cReset,
				faint(t.Name+" - "+t.Description),
			)
		}
		fmt.Println()
	}

	divider()

	count := 0
	for _, v := range selected {
		if v {
			count++
		}
	}
	if count > 0 {
		fmt.Printf(amber("  → %d tool(s) selected")+faint("  ·  press Enter to install\n"), count)
	} else {
		fmt.Println(faint("  → no tools selected yet"))
	}
	fmt.Println()
	fmt.Print(amber("  ❯ "))
}

func runSelector(tools []Tool) []Tool {
	// index of tools that are available
	available := []int{}
	for i, t := range tools {
		if t.Status == "available" {
			available = append(available, i)
		}
	}

	selected := map[int]bool{}
	r := bufio.NewReader(os.Stdin)

	for {
		drawMenu(tools, available, selected)
		input, _ := r.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))

		switch input {
		case "q", "quit", "exit":
			fmt.Println()
			fmt.Println(faint("  Bye."))
			fmt.Println()
			os.Exit(0)

		case "":
			chosen := []Tool{}
			for _, toolIdx := range available {
				if selected[toolIdx] {
					chosen = append(chosen, tools[toolIdx])
				}
			}
			if len(chosen) == 0 {
				fmt.Println()
				fmt.Println(red("  ✗  Nothing selected. Pick at least one."))
				pressEnter()
				continue
			}
			return chosen

		default:
			// accept one or more space-separated numbers
			for _, token := range strings.Fields(input) {
				var n int
				if _, err := fmt.Sscanf(token, "%d", &n); err != nil {
					continue
				}
				if n < 1 || n > len(available) {
					continue
				}
				toolIdx := available[n-1]
				selected[toolIdx] = !selected[toolIdx]
			}
		}
	}
}

// ── Confirm screen ────────────────────────────────────────────────────────────

func confirmInstall(chosen []Tool) bool {
	clearScreen()
	printLogo()
	fmt.Println(bold("  Ready to install:"))
	fmt.Println()
	for _, t := range chosen {
		fmt.Printf("  %s  %s\n", green("✓"), bold(t.Name))
		fmt.Printf("         %s\n", faint(t.Description))
		fmt.Println()
	}
	divider()
	fmt.Println()
	fmt.Print(amber("  Proceed? [Y/n] "))
	ans := readLine()
	return ans == "" || strings.ToLower(ans) == "y" || strings.ToLower(ans) == "yes"
}

// ── Install ───────────────────────────────────────────────────────────────────

type Result struct {
	Tool    Tool
	ToolDir string
	Err     error
}

func installTool(t Tool, repoRoot string) Result {
	toolDir := filepath.Join(repoRoot, t.Dir)

	// check the tool directory and go.mod exist inside the repo
	gomod := filepath.Join(toolDir, "go.mod")
	if _, err := os.Stat(gomod); os.IsNotExist(err) {
		return Result{Tool: t, Err: fmt.Errorf("go.mod not found in %s - tool may not be implemented yet", toolDir)}
	}

	fmt.Printf("  %s  %s\n", amber("↓"), bold(t.Name))

	// go mod download
	cmd := exec.Command("go", "mod", "download")
	cmd.Dir = toolDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return Result{Tool: t, Err: fmt.Errorf("go mod download failed: %w\n%s", err, string(out))}
	}

	fmt.Printf("  %s  %s %s\n", green("✓"), bold(t.Name), faint("ready at "+toolDir))
	return Result{Tool: t, ToolDir: toolDir}
}

// ── Print finish screen ───────────────────────────────────────────────────────

func printFinish(results []Result, repoRoot string) {
	clearScreen()
	printLogo()

	ok := []Result{}
	fail := []Result{}
	for _, r := range results {
		if r.Err == nil {
			ok = append(ok, r)
		} else {
			fail = append(fail, r)
		}
	}

	if len(fail) > 0 {
		fmt.Println(red("  Some tools failed:"))
		fmt.Println()
		for _, r := range fail {
			fmt.Printf("  %s  %s\n       %s\n\n", red("✗"), r.Tool.Name, faint(r.Err.Error()))
		}
		divider()
		fmt.Println()
	}

	if len(ok) == 0 {
		fmt.Println(red("  ✗  No tools installed."))
		os.Exit(1)
	}

	fmt.Println(green(bold("  ✓  Done! 🎉")))
	fmt.Println()

	// auth steps
	needGoogle := false
	needMicrosoft := false
	for _, r := range ok {
		for _, a := range r.Tool.RequiresAuth {
			if a == "google_oauth2" {
				needGoogle = true
			}
			if a == "microsoft_oauth2" {
				needMicrosoft = true
			}
		}
	}

	if needGoogle || needMicrosoft {
		fmt.Println(bold("  Authenticate your accounts first:"))
		fmt.Println()
		if needGoogle {
			fmt.Printf("  %s  %s\n", amber("→"), bold("Google (Gmail / Calendar)"))
			// find the first google tool dir
			for _, r := range ok {
				for _, a := range r.Tool.RequiresAuth {
					if a == "google_oauth2" {
						fmt.Printf("       %s\n\n", faint("cd "+r.ToolDir+" && go run . --auth"))
						break
					}
				}
			}
		}
		if needMicrosoft {
			fmt.Printf("  %s  %s\n", amber("→"), bold("Microsoft (Outlook)"))
			for _, r := range ok {
				for _, a := range r.Tool.RequiresAuth {
					if a == "microsoft_oauth2" {
						fmt.Printf("       %s\n\n", faint("cd "+r.ToolDir+" && go run . --auth"))
						break
					}
				}
			}
		}
		divider()
		fmt.Println()
	}

	// MCP config
	fmt.Println(bold("  Add to your editor MCP config:"))
	fmt.Println(faint("  Cursor → Settings › MCP   ·   Claude Code → ~/.claude/mcp.json"))
	fmt.Println()
	fmt.Println(faint("  {"))
	fmt.Println(faint(`    "mcpServers": {`))
	for i, r := range ok {
		comma := ","
		if i == len(ok)-1 {
			comma = ""
		}
		fmt.Printf(faint(`      "`)+cAmber+`claw-%s`+faint(`": {`)+"\n", r.Tool.ID)
		fmt.Println(faint(`        "command": "bash",`))
		fmt.Printf(faint(`        "args": ["-c", "`)+cGreen+`cd %s && go run . -- --mode mcp`+faint(`"]`)+"\n", r.ToolDir)
		fmt.Printf(faint(`      }%s`)+"\n", comma)
	}
	fmt.Println(faint("    }"))
	fmt.Println(faint("  }"))
	fmt.Println()
	divider()
	fmt.Println()

	// HTTP mode
	fmt.Println(bold("  For PicoClaw / OpenClaw / HTTP agents:"))
	fmt.Println()
	for _, r := range ok {
		varName := "CLAW_" + strings.ToUpper(r.Tool.ID) + "_URL"
		fmt.Printf("  %s=%s\n", amber(varName), faint(fmt.Sprintf("http://localhost:%d", r.Tool.HTTPPort)))
		fmt.Printf("  %s\n\n", faint(fmt.Sprintf("  # start: cd %s && go run . --mode http --port %d", r.ToolDir, r.Tool.HTTPPort)))
	}

	divider()
	fmt.Println()
	fmt.Printf("  %s  Repo: %s\n", green("✓"), faint(repoRoot))
	fmt.Println()
	fmt.Println(faint("  https://clawtools.dev  ·  github.com/arpit0515/claw-tools.dev"))
	fmt.Println()
}

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	repoFlag := flag.String("repo", "", "path to claw-tools repo root")
	flag.Parse()

	// resolve repo root: flag > env > walk up from binary location
	repoRoot := *repoFlag
	if repoRoot == "" {
		repoRoot = os.Getenv("CLAW_REPO")
	}
	if repoRoot == "" {
		// walk up from binary location looking for tools.json
		dir := filepath.Dir(os.Args[0])
		for i := 0; i < 5; i++ {
			if _, err := os.Stat(filepath.Join(dir, "tools.json")); err == nil {
				repoRoot = dir
				break
			}
			dir = filepath.Dir(dir)
		}
	}
	if repoRoot == "" {
		fmt.Println(red("  ✗  Cannot find repo root. Run ./install.sh from the claw-tools.dev directory."))
		os.Exit(1)
	}

	tools, err := loadTools(repoRoot)
	if err != nil {
		fmt.Println(red("  ✗  " + err.Error()))
		os.Exit(1)
	}

	// select
	chosen := runSelector(tools)

	// confirm
	if !confirmInstall(chosen) {
		fmt.Println(faint("\n  Aborted."))
		os.Exit(0)
	}

	// install
	clearScreen()
	printLogo()
	fmt.Println(bold("  Installing..."))
	fmt.Println()

	results := make([]Result, 0, len(chosen))
	for _, t := range chosen {
		results = append(results, installTool(t, repoRoot))
	}

	fmt.Println()
	pressEnter()
	printFinish(results, repoRoot)
}

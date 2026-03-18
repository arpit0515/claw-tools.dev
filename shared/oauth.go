package shared

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// ── Constants ─────────────────────────────────────────────────────────────────

const (
	TokensDir  = ".claw/tokens"
	ConfigDir  = ".claw/config"
	CredsFile  = "google_credentials.json"
	CallbackPort = "3455"
)

// ── Account ───────────────────────────────────────────────────────────────────

// Account represents a connected Google account
type Account struct {
	Email     string    `json:"email"`
	TokenFile string    `json:"token_file"`
	AddedAt   time.Time `json:"added_at"`
}

// ── Paths ─────────────────────────────────────────────────────────────────────

func HomeDir() string {
	h, _ := os.UserHomeDir()
	return h
}

func TokensPath() string {
	return filepath.Join(HomeDir(), TokensDir)
}

func CredsPath() string {
	return filepath.Join(HomeDir(), ConfigDir, CredsFile)
}

// TokenPathForAccount returns the encrypted token file path for a given email
// e.g. ~/.claw/tokens/arpit@gmail.com.enc
func TokenPathForAccount(email string) string {
	safe := strings.ReplaceAll(email, "/", "_")
	return filepath.Join(TokensPath(), safe+".enc")
}

// ── Encryption ────────────────────────────────────────────────────────────────
// Tokens are encrypted at rest using a key derived from the machine's hostname
// This is lightweight protection — not HSM grade, but stops casual file reads

func machineKey() ([]byte, error) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "claw-default"
	}
	// Add a fixed salt so the key is always deterministic per machine
	raw := "claw-token-key:" + hostname
	hash := sha256.Sum256([]byte(raw))
	return hash[:], nil
}

func encrypt(plaintext []byte) ([]byte, error) {
	key, err := machineKey()
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func decrypt(ciphertext []byte) ([]byte, error) {
	key, err := machineKey()
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// ── Token storage ─────────────────────────────────────────────────────────────

func SaveToken(email string, tok *oauth2.Token) error {
	if err := os.MkdirAll(TokensPath(), 0700); err != nil {
		return fmt.Errorf("cannot create tokens dir: %w", err)
	}
	data, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return err
	}
	enc, err := encrypt(data)
	if err != nil {
		return fmt.Errorf("encryption failed: %w", err)
	}
	path := TokenPathForAccount(email)
	if err := os.WriteFile(path, enc, 0600); err != nil {
		return fmt.Errorf("cannot write token: %w", err)
	}
	return nil
}

func LoadToken(email string) (*oauth2.Token, error) {
	path := TokenPathForAccount(email)
	enc, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("account %q not found — run: go run . --auth --account %s", email, email)
		}
		return nil, err
	}
	data, err := decrypt(enc)
	if err != nil {
		return nil, fmt.Errorf("token decryption failed (token may be corrupt): %w", err)
	}
	var tok oauth2.Token
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, fmt.Errorf("invalid token file: %w", err)
	}
	return &tok, nil
}

func DeleteToken(email string) error {
	path := TokenPathForAccount(email)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ── Account listing ───────────────────────────────────────────────────────────

// ListAccounts returns all connected Google accounts by scanning the tokens dir
func ListAccounts() ([]Account, error) {
	entries, err := os.ReadDir(TokensPath())
	if err != nil {
		if os.IsNotExist(err) {
			return []Account{}, nil
		}
		return nil, err
	}
	accounts := []Account{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".enc") {
			continue
		}
		email := strings.TrimSuffix(e.Name(), ".enc")
		info, _ := e.Info()
		addedAt := time.Time{}
		if info != nil {
			addedAt = info.ModTime()
		}
		accounts = append(accounts, Account{
			Email:     email,
			TokenFile: filepath.Join(TokensPath(), e.Name()),
			AddedAt:   addedAt,
		})
	}
	return accounts, nil
}

// DefaultAccount returns the first connected account, or error if none
func DefaultAccount() (string, error) {
	accounts, err := ListAccounts()
	if err != nil {
		return "", err
	}
	if len(accounts) == 0 {
		return "", fmt.Errorf("no accounts connected — run: go run . --auth")
	}
	return accounts[0].Email, nil
}

// ── OAuth config ──────────────────────────────────────────────────────────────

// NewOAuthConfig loads credentials and builds an oauth2.Config for the given scopes
func NewOAuthConfig(scopes ...string) (*oauth2.Config, error) {
	data, err := os.ReadFile(CredsPath())
	if err != nil {
		return nil, fmt.Errorf(
			"credentials not found at %s\n"+
				"  1. Go to https://console.cloud.google.com → APIs & Services → Credentials\n"+
				"  2. Create OAuth 2.0 Client ID → Desktop app\n"+
				"  3. Download JSON → save as %s",
			CredsPath(), CredsPath(),
		)
	}
	cfg, err := google.ConfigFromJSON(data, scopes...)
	if err != nil {
		return nil, fmt.Errorf("invalid credentials file: %w", err)
	}
	return cfg, nil
}

// ── Auth flow ─────────────────────────────────────────────────────────────────

// AuthResult is returned after a successful OAuth flow
type AuthResult struct {
	Email string
	Token *oauth2.Token
}

// RunAuthFlow performs the OAuth flow via a local callback server on port 3455.
// It opens the browser, waits for Google to redirect back with the code,
// exchanges it for tokens, fetches the user's email, and saves the token.
func RunAuthFlow(cfg *oauth2.Config) (*AuthResult, error) {
	// Override redirect URI to our local callback server
	cfg.RedirectURL = "http://localhost:" + CallbackPort + "/oauth/callback"

	state := randomState()
	authURL := cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)

	// Channel to receive the auth code from the callback handler
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	// Start temporary local HTTP server for the callback
	mux := http.NewServeMux()
	srv := &http.Server{Addr: ":" + CallbackPort, Handler: mux}

	mux.HandleFunc("/oauth/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.Error(w, "Invalid state", http.StatusBadRequest)
			errCh <- fmt.Errorf("state mismatch — possible CSRF")
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			errMsg := r.URL.Query().Get("error")
			http.Error(w, "No code received", http.StatusBadRequest)
			errCh <- fmt.Errorf("no code in callback: %s", errMsg)
			return
		}
		// Friendly success page
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, successPage())
		codeCh <- code
	})

	// Start server in background
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("callback server error: %w", err)
		}
	}()

	// Open browser
	fmt.Println("\n🔐 Opening browser for Google authorization...")
	fmt.Println("   If it doesn't open automatically, visit:\n")
	fmt.Println("   " + authURL + "\n")
	openBrowser(authURL)

	// Wait for code or error (60 second timeout)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	defer srv.Shutdown(ctx)

	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, fmt.Errorf("timed out waiting for authorization (60s)")
	}

	// Exchange code for token
	tok, err := cfg.Exchange(context.Background(), code)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}

	// Fetch the email address from Google userinfo
	email, err := fetchUserEmail(tok)
	if err != nil {
		return nil, fmt.Errorf("could not fetch account email: %w", err)
	}

	// Save token encrypted
	if err := SaveToken(email, tok); err != nil {
		return nil, err
	}

	return &AuthResult{Email: email, Token: tok}, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func fetchUserEmail(tok *oauth2.Token) (string, error) {
	client := oauth2.NewClient(context.Background(),
		oauth2.StaticTokenSource(tok))
	resp, err := client.Get("https://www.googleapis.com/oauth2/v3/userinfo")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var info struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", err
	}
	if info.Email == "" {
		return "", fmt.Errorf("empty email in userinfo response")
	}
	return info.Email, nil
}

func randomState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func openBrowser(url string) {
	// Try common openers across platforms
	for _, cmd := range [][]string{
		{"xdg-open", url},      // Linux
		{"open", url},          // macOS
		{"cmd", "/c", "start", url}, // Windows
	} {
		if err := runSilent(cmd[0], cmd[1:]...); err == nil {
			return
		}
	}
}

func runSilent(name string, args ...string) error {
	// exec without importing os/exec in this file — use a small helper
	// we import it at the top of this file
	cmd := execCommand(name, args...)
	return cmd.Run()
}

func successPage() string {
	return `<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
  <title>ClawTools — Connected</title>
  <style>
    body { font-family: -apple-system, sans-serif; background: #0f1117; color: #e8eaf6;
           display: flex; align-items: center; justify-content: center; min-height: 100vh; margin: 0; }
    .card { background: #1a1d27; border: 1px solid #2e3350; border-radius: 12px;
            padding: 40px 48px; text-align: center; max-width: 400px; }
    .icon { font-size: 48px; margin-bottom: 16px; }
    h1 { font-size: 22px; font-weight: 600; margin: 0 0 8px; color: #00d4aa; }
    p  { color: #8b90b0; font-size: 14px; margin: 0; }
  </style>
</head>
<body>
  <div class="card">
    <div class="icon">🦞</div>
    <h1>Account connected!</h1>
    <p>You can close this tab and return to the terminal.</p>
  </div>
</body>
</html>`
}

// GetAuthenticatedClient returns an HTTP client for a given account,
// automatically refreshing the token if expired
func GetAuthenticatedClient(cfg *oauth2.Config, email string) (*http.Client, error) {
	tok, err := LoadToken(email)
	if err != nil {
		return nil, err
	}
	// oauth2 Transport auto-refreshes expired tokens using the refresh token
	tokenSource := cfg.TokenSource(context.Background(), tok)

	// Persist refreshed token back to disk
	newTok, err := tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("token refresh failed for %s: %w", email, err)
	}
	if newTok.AccessToken != tok.AccessToken {
		if err := SaveToken(email, newTok); err != nil {
			// Non-fatal — client still works with in-memory token
			fmt.Fprintf(os.Stderr, "warning: could not persist refreshed token: %v\n", err)
		}
	}

	return oauth2.NewClient(context.Background(), tokenSource), nil
}

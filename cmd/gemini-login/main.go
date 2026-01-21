// Command gemini-login authenticates with Google OAuth for Gemini access.
//
// Usage:
//
//	go run cmd/gemini-login/main.go
//
// This opens a browser for Google OAuth login and saves credentials to
// ~/.config/vango/gemini-oauth-credentials.json
//
// The project ID is auto-discovered from your GCP account.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/vango-go/vai/pkg/core/providers/gemini_oauth"
)

func main() {
	fmt.Println("Starting Gemini OAuth login...")
	fmt.Println()

	// Start the OAuth flow (empty project ID = auto-discover)
	resultChan, authURL, err := gemini_oauth.StartLogin("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start login: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Opening browser for Google OAuth...")
	fmt.Println()
	fmt.Println("If browser doesn't open, visit:")
	fmt.Println(authURL)
	fmt.Println()

	// Try to open browser
	openBrowser(authURL)

	fmt.Println("Waiting for OAuth callback...")

	// Wait for result (with 5 minute timeout)
	select {
	case result := <-resultChan:
		if result.Error != nil {
			fmt.Fprintf(os.Stderr, "OAuth failed: %v\n", result.Error)
			os.Exit(1)
		}

		// Save credentials
		store := gemini_oauth.NewFileCredentialsStore("")
		if err := store.Save(result.Credentials); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to save credentials: %v\n", err)
			os.Exit(1)
		}

		fmt.Println()
		fmt.Println("Success! Credentials saved.")
		fmt.Printf("Project ID: %s (auto-discovered)\n", result.Credentials.ProjectID)
		fmt.Printf("Expires: %s\n", result.Credentials.Expiry.Format(time.RFC3339))

	case <-time.After(5 * time.Minute):
		fmt.Fprintf(os.Stderr, "Timeout waiting for OAuth callback\n")
		os.Exit(1)
	}
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return
	}
	_ = cmd.Start()
}

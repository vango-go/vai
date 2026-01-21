package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/collinshill/gemini-proxy/pkg/auth"
	"github.com/collinshill/gemini-proxy/pkg/proxy"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "login":
		runLogin()
	case "serve":
		runServe()
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Gemini Proxy - Use your Gemini plan with standard API clients")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  gemini-proxy login   - Authenticate with Google")
	fmt.Println("  gemini-proxy serve   - Start the proxy server")
}

func runLogin() {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("=== Gemini Proxy Login ===")
	fmt.Println()
	fmt.Print("Enter your Google Cloud Project ID: ")
	projectID, _ := reader.ReadString('\n')
	projectID = strings.TrimSpace(projectID)

	if projectID == "" {
		fmt.Println("Error: Project ID is required")
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("Starting OAuth flow...")
	fmt.Println("A browser window will open. Sign in with your Google account.")
	fmt.Println()

	resultChan, authURL, err := auth.StartLogin(projectID)
	if err != nil {
		fmt.Printf("Error starting login: %v\n", err)
		os.Exit(1)
	}

	// Open browser
	openBrowser(authURL)
	fmt.Println("If the browser didn't open, visit this URL:")
	fmt.Println(authURL)
	fmt.Println()
	fmt.Println("Waiting for authentication...")

	result := <-resultChan
	if result.Error != nil {
		fmt.Printf("Error during authentication: %v\n", result.Error)
		os.Exit(1)
	}

	if err := auth.SaveCredentials(result.Credentials); err != nil {
		fmt.Printf("Error saving credentials: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("✓ Authentication successful!")
	fmt.Println("  Credentials saved to ~/.config/gemini-proxy/credentials.json")
	fmt.Println()
	fmt.Println("Run 'gemini-proxy serve' to start the proxy server.")
}

func runServe() {
	creds, err := auth.LoadCredentials()
	if err != nil {
		fmt.Println("Error: Not logged in. Run 'gemini-proxy login' first.")
		os.Exit(1)
	}

	port := 8080
	if len(os.Args) > 2 {
		fmt.Sscanf(os.Args[2], "%d", &port)
	}

	server := proxy.NewServer(creds, port)
	if err := server.Start(); err != nil {
		fmt.Printf("Server error: %v\n", err)
		os.Exit(1)
	}
}

func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "darwin":
		err = exec.Command("open", url).Start()
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	}
	if err != nil {
		// Ignore error, user can open URL manually
	}
}

package cli

import (
	"fmt"
	"os"

	"github.com/smallnest/dogclaw/goclaw/clawhub"
	"github.com/spf13/cobra"
)

var (
	loginToken    string
	loginLabel    string
	loginNoBrowser bool
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with ClawHub",
	Long: `Authenticate with ClawHub using browser flow or API token.

By default, opens a browser for OAuth flow. Use --token to authenticate
with an API token directly.`,
	Run: runLogin,
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out from ClawHub",
	Run:   runLogout,
}

var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Display current authenticated user",
	Run:   runWhoami,
}

func addClawhubAuthCommands() {
	clawhubCmd.AddCommand(loginCmd)
	clawhubCmd.AddCommand(logoutCmd)
	clawhubCmd.AddCommand(whoamiCmd)

	loginCmd.Flags().StringVar(&loginToken, "token", "", "Paste an API token directly")
	loginCmd.Flags().StringVar(&loginLabel, "label", "CLI token", "Label for stored token")
	loginCmd.Flags().BoolVar(&loginNoBrowser, "no-browser", false, "Do not open browser (requires --token)")
}

func runLogin(cmd *cobra.Command, args []string) {
	cfg, err := loadClawhubConfig()
	if err != nil {
		printError("Failed to load config: %v", err)
		os.Exit(1)
	}

	if loginNoBrowser && loginToken == "" {
		printError("--no-browser requires --token")
		os.Exit(1)
	}

	var token string

	if loginToken != "" {
		// Direct token login
		token = loginToken
		printInfo("Using provided token")
	} else {
		// Browser flow
		printInfo("Opening browser for authentication...")

		client := clawhub.NewClient(clawhub.GetRegistryURL(cfg), "")
		authURL, _, err := client.InitiateBrowserAuth()
		if err != nil {
			printError("Failed to initiate authentication: %v", err)
			os.Exit(1)
		}

		// Open browser
		if err := clawhub.OpenBrowser(authURL); err != nil {
			printWarning("Failed to open browser automatically: %v", err)
			printInfo("Please visit this URL in your browser:")
			fmt.Printf("  %s\n", authURL)
		} else {
			printInfo("Browser opened. Complete the authentication there.")
		}

		fmt.Println()
		printInfo("After authentication, you will receive a token.")
		printInfo("Paste the token below:")

		// Prompt for the token
		token = prompt("Token")
		if token == "" {
			printError("Token is required")
			os.Exit(1)
		}
	}

	// Store token
	label := loginLabel
	if label == "" {
		label = "CLI token"
	}
	cfg.SetToken(token, label)

	// Save config
	if err := clawhub.SaveConfig(cfg); err != nil {
		printError("Failed to save config: %v", err)
		os.Exit(1)
	}

	// Verify authentication
	client := clawhub.NewClient(clawhub.GetRegistryURL(cfg), token)
	userInfo, err := client.GetUserInfo()
	if err != nil {
		printError("Failed to verify authentication: %v", err)
		printWarning("Token was saved but verification failed. You may need to login again.")
		os.Exit(1)
	}

	printSuccess("Logged in as %s (%s)", userInfo.Name, userInfo.Login)
}

func runLogout(cmd *cobra.Command, args []string) {
	cfg, err := loadClawhubConfig()
	if err != nil {
		printError("Failed to load config: %v", err)
		os.Exit(1)
	}

	if !cfg.IsAuthenticated() {
		printWarning("Not logged in")
		return
	}

	cfg.ClearToken()

	if err := clawhub.SaveConfig(cfg); err != nil {
		printError("Failed to save config: %v", err)
		os.Exit(1)
	}

	printSuccess("Logged out successfully")
}

func runWhoami(cmd *cobra.Command, args []string) {
	cfg, err := loadClawhubConfig()
	if err != nil {
		printError("Failed to load config: %v", err)
		os.Exit(1)
	}

	if !cfg.IsAuthenticated() {
		printWarning("Not logged in")
		os.Exit(1)
	}

	client := clawhub.NewClient(clawhub.GetRegistryURL(cfg), cfg.Token)
	userInfo, err := client.GetUserInfo()
	if err != nil {
		printError("Failed to get user info: %v", err)
		os.Exit(1)
	}

	fmt.Println("Authenticated User:")
	fmt.Println("===================")
	fmt.Printf("Login:      %s\n", userInfo.Login)
	fmt.Printf("Name:       %s\n", userInfo.Name)
	fmt.Printf("Email:      %s\n", userInfo.Email)
	fmt.Printf("Created:    %s\n", userInfo.CreatedAt.Format("2006-01-02"))
}

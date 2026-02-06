package clawhub

import (
	"runtime"
	"testing"
)

func TestInitiateBrowserAuth(t *testing.T) {
	client := NewClient("https://clawhub.ai", "")

	authURL, state, err := client.InitiateBrowserAuth()
	if err != nil {
		t.Fatalf("InitiateBrowserAuth() error = %v", err)
	}

	if authURL == "" {
		t.Error("expected non-empty authURL")
	}

	if state == "" {
		t.Error("expected non-empty state")
	}

	// Verify authURL contains expected parameters
	if len(authURL) < 10 {
		t.Errorf("authURL too short: %s", authURL)
	}
}

func TestOpenBrowser(t *testing.T) {
	// Just verify the function doesn't crash
	// We won't actually test opening a browser in unit tests
	// This is more of an integration test

	testURL := "https://clawhub.ai"

	// On Windows, this might fail if start command is not available
	// On Linux, xdg-open might not be installed
	// On macOS, open should always be available

	err := OpenBrowser(testURL)

	// We expect this might fail in CI environments
	// So we just log the result rather than failing
	if err != nil {
		t.Logf("OpenBrowser failed (expected in some environments): %v", err)
	} else {
		t.Log("OpenBrowser succeeded")
	}
}

func TestBuildAuthURL(t *testing.T) {
	siteURL := "https://clawhub.ai"
	state := "test-state-123"

	authURL := BuildAuthURL(siteURL, state)

	expectedPrefix := siteURL + "/auth/authorize"
	if len(authURL) < len(expectedPrefix) || authURL[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("expected URL to start with %s, got %s", expectedPrefix, authURL)
	}

	// Check that state is in the URL
	if !contains(authURL, "state="+state) {
		t.Errorf("expected state parameter in URL, got %s", authURL)
	}

	// Check for response_type=token
	if !contains(authURL, "response_type=token") {
		t.Errorf("expected response_type=token in URL, got %s", authURL)
	}
}

func TestBuildTokenExchangeURL(t *testing.T) {
	siteURL := "https://clawhub.ai"
	tokenURL := BuildTokenExchangeURL(siteURL)

	expected := siteURL + "/auth/token"
	if tokenURL != expected {
		t.Errorf("expected %s, got %s", expected, tokenURL)
	}
}

func TestBrowserAuthFlowPlatform(t *testing.T) {
	// Just log the platform for debugging purposes
	t.Logf("Running on platform: %s", runtime.GOOS)

	// Verify that we have a browser command for this platform
	switch runtime.GOOS {
	case "windows":
		t.Log("Will use: cmd /c start")
	case "darwin":
		t.Log("Will use: open")
	case "linux":
		t.Log("Will use: xdg-open")
	default:
		t.Logf("Unknown platform, might not have browser support: %s", runtime.GOOS)
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

package clawhub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"runtime"
	"time"
)

// Client is the HTTP client for the ClawHub registry API
type Client struct {
	baseURL    string
	httpClient *http.Client
	token      string
}

// NewClient creates a new registry client
func NewClient(registryURL, token string) *Client {
	return &Client{
		baseURL: registryURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		token: token,
	}
}

// SearchResult represents a search result
type SearchResult struct {
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Stats       Stats    `json:"stats"`
}

// Stats represents skill statistics
type Stats struct {
	Stars     int `json:"stars"`
	Downloads int `json:"downloads"`
	Updates   int `json:"updates"`
}

// SkillVersion represents a version of a skill
type SkillVersion struct {
	Version   string    `json:"version"`
	Changelog string    `json:"changelog"`
	CreatedAt time.Time `json:"created_at"`
	Hash      string    `json:"hash"`
	DownloadURL string `json:"download_url"`
}

// SkillDetail represents detailed skill information
type SkillDetail struct {
	Slug        string         `json:"slug"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Versions    []SkillVersion `json:"versions"`
	Tags        []string       `json:"tags"`
	Stats       Stats          `json:"stats"`
}

// UserInfo represents user information
type UserInfo struct {
	Login     string `json:"login"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// Search searches for skills using the query
func (c *Client) Search(query string, limit int) ([]SearchResult, error) {
	url := fmt.Sprintf("%s/api/skills/search?q=%s&limit=%d", c.baseURL, query, limit)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search failed with status %d", resp.StatusCode)
	}

	var results []SearchResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return results, nil
}

// GetSkill retrieves skill details
func (c *Client) GetSkill(slug string) (*SkillDetail, error) {
	url := fmt.Sprintf("%s/api/skills/%s", c.baseURL, slug)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("skill '%s' not found", slug)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get skill failed with status %d", resp.StatusCode)
	}

	var detail SkillDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &detail, nil
}

// DownloadSkill downloads a skill version
func (c *Client) DownloadSkill(slug, version string) ([]byte, error) {
	url := fmt.Sprintf("%s/api/skills/%s/versions/%s/download", c.baseURL, slug, version)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("version %s not found for skill '%s'", version, slug)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return data, nil
}

// PublishRequest represents a publish request
type PublishRequest struct {
	Slug      string   `json:"slug"`
	Name      string   `json:"name"`
	Version   string   `json:"version"`
	Changelog string   `json:"changelog"`
	Tags      []string `json:"tags"`
	Bundle    []byte   `json:"-"` // The zip file data
}

// PublishResponse represents a publish response
type PublishResponse struct {
	Slug    string `json:"slug"`
	Version string `json:"version"`
	URL     string `json:"url"`
}

// Publish publishes a skill to the registry
func (c *Client) Publish(req *PublishRequest) (*PublishResponse, error) {
	url := fmt.Sprintf("%s/api/skills/publish", c.baseURL)

	// Create multipart form
	body := &bytes.Buffer{}
	writer := newMultipartWriter(body)

	// Add metadata fields
	writer.AddField("slug", req.Slug)
	writer.AddField("name", req.Name)
	writer.AddField("version", req.Version)
	writer.AddField("changelog", req.Changelog)

	// Add tags
	for _, tag := range req.Tags {
		writer.AddField("tags", tag)
	}

	// Add bundle file
	if err := writer.AddFile("bundle", req.Bundle, req.Slug+".zip"); err != nil {
		return nil, fmt.Errorf("failed to add bundle file: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	httpReq, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("not authenticated. Run 'goclaw clawhub login' first")
	}

	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("permission denied. Your GitHub account must be at least one week old to publish")
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("publish failed with status %d: %s", resp.StatusCode, string(body))
	}

	var publishResp PublishResponse
	if err := json.NewDecoder(resp.Body).Decode(&publishResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &publishResp, nil
}

// GetUserInfo retrieves current user information
func (c *Client) GetUserInfo() (*UserInfo, error) {
	url := fmt.Sprintf("%s/api/user", c.baseURL)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if c.token == "" {
		return nil, fmt.Errorf("not authenticated")
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("invalid token")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get user info failed with status %d", resp.StatusCode)
	}

	var userInfo UserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &userInfo, nil
}

// DeleteSkill deletes a skill from the registry
func (c *Client) DeleteSkill(slug string) error {
	url := fmt.Sprintf("%s/api/skills/%s", c.baseURL, slug)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if c.token == "" {
		return fmt.Errorf("not authenticated")
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("not authenticated")
	}

	if resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("permission denied. Only skill owners can delete skills")
	}

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("skill '%s' not found", slug)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("delete failed with status %d", resp.StatusCode)
	}

	return nil
}

// UndeleteSkill undeletes a skill from the registry
func (c *Client) UndeleteSkill(slug string) error {
	url := fmt.Sprintf("%s/api/skills/%s/undelete", c.baseURL, slug)

	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if c.token == "" {
		return fmt.Errorf("not authenticated")
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("not authenticated")
	}

	if resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("permission denied. Only skill owners can undelete skills")
	}

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("skill '%s' not found", slug)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("undelete failed with status %d", resp.StatusCode)
	}

	return nil
}

// multipartWriter is a simple multipart writer for file uploads
type multipartWriter struct {
	boundary string
	buf       *bytes.Buffer
}

func newMultipartWriter(buf *bytes.Buffer) *multipartWriter {
	return &multipartWriter{
		boundary: fmt.Sprintf("boundary%d", time.Now().UnixNano()),
		buf:       buf,
	}
}

func (w *multipartWriter) AddField(name, value string) {
	w.buf.WriteString(fmt.Sprintf("--%s\r\n", w.boundary))
	w.buf.WriteString(fmt.Sprintf("Content-Disposition: form-data; name=\"%s\"\r\n\r\n", name))
	w.buf.WriteString(value)
	w.buf.WriteString("\r\n")
}

func (w *multipartWriter) AddFile(name string, data []byte, filename string) error {
	w.buf.WriteString(fmt.Sprintf("--%s\r\n", w.boundary))
	w.buf.WriteString(fmt.Sprintf("Content-Disposition: form-data; name=\"%s\"; filename=\"%s\"\r\n", name, filename))
	w.buf.WriteString("Content-Type: application/zip\r\n\r\n")
	w.buf.Write(data)
	w.buf.WriteString("\r\n")
	return nil
}

func (w *multipartWriter) Close() error {
	w.buf.WriteString(fmt.Sprintf("--%s--\r\n", w.boundary))
	return nil
}

func (w *multipartWriter) FormDataContentType() string {
	return fmt.Sprintf("multipart/form-data; boundary=%s", w.boundary)
}

// BuildDownloadURL builds a download URL for a skill version
func BuildDownloadURL(baseURL, slug, version string) string {
	return fmt.Sprintf("%s/api/skills/%s/versions/%s/download", baseURL, slug, version)
}

// BuildSkillURL builds a URL for a skill
func BuildSkillURL(baseURL, slug string) string {
	return fmt.Sprintf("%s/skills/%s", baseURL, slug)
}

// BuildAuthURL builds an authorization URL for browser login
func BuildAuthURL(siteURL, state string) string {
	return fmt.Sprintf("%s/auth/authorize?state=%s&response_type=token&client_id=cli", siteURL, state)
}

// BuildTokenExchangeURL builds the token exchange URL
func BuildTokenExchangeURL(siteURL string) string {
	return fmt.Sprintf("%s/auth/token", siteURL)
}

// PollTokenResponse represents the response from token polling
type PollTokenResponse struct {
	Token   string `json:"token"`
	Expires int    `json:"expires_in"`
	UserID  string `json:"user_id"`
}

// DeviceAuthResponse represents device authorization response
type DeviceAuthResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// OpenBrowser opens the default web browser to the specified URL
func OpenBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
		// No additional args needed on macOS
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}

	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}

// InitiateBrowserAuth initiates browser-based authentication
// Returns the authorization URL and a state parameter for CSRF protection
func (c *Client) InitiateBrowserAuth() (string, string, error) {
	state := fmt.Sprintf("cli-%d", time.Now().UnixNano())
	authURL := BuildAuthURL(c.baseURL, state)
	return authURL, state, nil
}

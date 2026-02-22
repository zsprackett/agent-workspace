package claudeusage

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
)

const (
	usageURL  = "https://api.anthropic.com/api/oauth/usage"
	userAgent = "claude-code/2.0.32"
	betaFlag  = "oauth-2025-04-20"
)

// GetToken retrieves the Claude Code OAuth access token from macOS Keychain.
func GetToken() (string, error) {
	out, err := exec.Command("security", "find-generic-password", "-s", "Claude Code-credentials", "-w").Output()
	if err != nil {
		return "", fmt.Errorf("keychain lookup: %w", err)
	}
	raw := strings.TrimSpace(string(out))
	var creds struct {
		ClaudeAiOauth struct {
			AccessToken string `json:"accessToken"`
		} `json:"claudeAiOauth"`
	}
	if err := json.Unmarshal([]byte(raw), &creds); err != nil {
		return "", fmt.Errorf("parse credentials: %w", err)
	}
	if creds.ClaudeAiOauth.AccessToken == "" {
		return "", fmt.Errorf("accessToken not found in credentials")
	}
	return creds.ClaudeAiOauth.AccessToken, nil
}

// FetchUsage retrieves the current Claude Code usage statistics from the Anthropic API.
func FetchUsage() (*UsageResponse, error) {
	token, err := GetToken()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", usageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", betaFlag)
	req.Header.Set("User-Agent", userAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var usage UsageResponse
	if err := json.Unmarshal(body, &usage); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &usage, nil
}

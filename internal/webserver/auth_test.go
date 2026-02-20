package webserver_test

import (
	"testing"
	"time"

	"github.com/zsprackett/agent-workspace/internal/webserver"
)

func TestIssueAndValidateAccessToken(t *testing.T) {
	secret := "test-secret"
	token, err := webserver.IssueAccessToken(secret, "alice", time.Hour)
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	username, err := webserver.ValidateAccessToken(secret, token)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}
	if username != "alice" {
		t.Errorf("expected alice, got %s", username)
	}
}

func TestValidateAccessToken_Expired(t *testing.T) {
	secret := "test-secret"
	token, _ := webserver.IssueAccessToken(secret, "alice", -time.Second)
	_, err := webserver.ValidateAccessToken(secret, token)
	if err == nil {
		t.Error("expected error for expired token")
	}
}

func TestValidateAccessToken_WrongSecret(t *testing.T) {
	token, _ := webserver.IssueAccessToken("secret-a", "alice", time.Hour)
	_, err := webserver.ValidateAccessToken("secret-b", token)
	if err == nil {
		t.Error("expected error for wrong secret")
	}
}

func TestGenerateRefreshToken(t *testing.T) {
	tok1, err := webserver.GenerateRefreshToken()
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}
	tok2, _ := webserver.GenerateRefreshToken()
	if tok1 == tok2 {
		t.Error("expected unique tokens")
	}
	if len(tok1) != 64 { // 32 bytes hex = 64 chars
		t.Errorf("expected 64 char token, got %d", len(tok1))
	}
}

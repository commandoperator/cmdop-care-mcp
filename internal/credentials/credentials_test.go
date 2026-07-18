package credentials_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	keyring "github.com/zalando/go-keyring"

	"github.com/commandoperator/cmdop-care-mcp/internal/credentials"
)

func resetKeyring(t *testing.T) {
	t.Helper()
	keyring.MockInit()
}

func TestRelayToken_NotEnrolled(t *testing.T) {
	resetKeyring(t)
	_, err := credentials.RelayToken()
	if !errors.Is(err, credentials.ErrNotEnrolled) {
		t.Fatalf("expected ErrNotEnrolled on an empty keyring, got %v", err)
	}
}

func TestRelayToken_HappyPath(t *testing.T) {
	resetKeyring(t)
	blob, _ := json.Marshal(map[string]any{"access_token": "cmdop_abc123"})
	if err := keyring.Set("cmdop", "token_prod", string(blob)); err != nil {
		t.Fatalf("seed keyring: %v", err)
	}
	tok, err := credentials.RelayToken()
	if err != nil {
		t.Fatalf("RelayToken: %v", err)
	}
	if tok != "cmdop_abc123" {
		t.Fatalf("expected cmdop_abc123, got %q", tok)
	}
}

func TestRelayToken_Expired(t *testing.T) {
	resetKeyring(t)
	past := time.Now().Add(-1 * time.Hour)
	blob, _ := json.Marshal(map[string]any{"access_token": "cmdop_abc123", "expires_at": past})
	if err := keyring.Set("cmdop", "token_prod", string(blob)); err != nil {
		t.Fatalf("seed keyring: %v", err)
	}
	_, err := credentials.RelayToken()
	if !errors.Is(err, credentials.ErrExpiredToken) {
		t.Fatalf("expected ErrExpiredToken, got %v", err)
	}
}

func TestRelayToken_ZeroExpiryNeverExpires(t *testing.T) {
	resetKeyring(t)
	blob, _ := json.Marshal(map[string]any{"access_token": "cmdop_abc123"}) // expires_at omitted -> zero time
	if err := keyring.Set("cmdop", "token_prod", string(blob)); err != nil {
		t.Fatalf("seed keyring: %v", err)
	}
	tok, err := credentials.RelayToken()
	if err != nil {
		t.Fatalf("expected zero ExpiresAt to mean never-expires, got error: %v", err)
	}
	if tok != "cmdop_abc123" {
		t.Fatalf("unexpected token: %q", tok)
	}
}

func TestMachinePIN_NoneCached(t *testing.T) {
	resetKeyring(t)
	_, err := credentials.MachinePIN("some-machine-id")
	if !errors.Is(err, credentials.ErrNoPINCached) {
		t.Fatalf("expected ErrNoPINCached, got %v", err)
	}
}

func TestMachinePIN_EmptyMachineID(t *testing.T) {
	resetKeyring(t)
	_, err := credentials.MachinePIN("")
	if !errors.Is(err, credentials.ErrNoPINCached) {
		t.Fatalf("expected ErrNoPINCached for an empty machine ID, got %v", err)
	}
}

func TestMachinePIN_HappyPath(t *testing.T) {
	resetKeyring(t)
	if err := keyring.Set("cmdop", "machinepw:id:machine-1", "1234"); err != nil {
		t.Fatalf("seed keyring: %v", err)
	}
	pin, err := credentials.MachinePIN("machine-1")
	if err != nil {
		t.Fatalf("MachinePIN: %v", err)
	}
	if pin != "1234" {
		t.Fatalf("expected 1234, got %q", pin)
	}
}

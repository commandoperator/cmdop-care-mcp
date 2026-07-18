// Package credentials resolves this artifact's ONLY supported credential
// path: the host-keyring passthrough decided in the Phase 1 security review
// (security-architecture-2026-07-18.md, "Decisions closed before Phase 2
// implementation"). cmdop-care does NOT implement enrollment, does NOT accept
// a relay token via an environment variable or a mounted secret as a primary
// path, and does NOT talk to Django or any OAuth/device-flow endpoint. It
// requires the host machine to already be enrolled via the main `cmdop` CLI
// (`cmdop enroll <enrollment-password>`), and reads that CLI's own
// keyring-backed relay token and per-machine PIN store directly.
//
// This intentionally duplicates (rather than imports) two narrow slices of
// the private cmdop_go monorepo's storage format, so this module has zero
// dependency on the private repo's internal/security/auth or
// internal/security/machinepw packages (which are themselves entangled with
// internal/lifecycle/config, internal/foundation, and other product-internal
// packages not appropriate to vendor into a public artifact):
//
//   - relay OAuth/enrollment Bearer token: OS keyring service "cmdop",
//     key "token_prod" (production mode; see modeKey below), a JSON blob
//     compatible with the private repo's auth.Token shape (only the fields
//     this artifact needs are decoded — access_token, expires_at).
//   - per-machine connection PIN: OS keyring service "cmdop",
//     key "machinepw:id:<machineID>" — an exact match of the private repo's
//     internal/security/machinepw key format, so a PIN saved by the real
//     `cmdop` CLI (e.g. via `cmdop connect`) is read here unmodified.
//
// Nothing in this package ever WRITES a token or a PIN — read-only by
// construction, matching this artifact's read-only tool contract.
package credentials

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	keyring "github.com/zalando/go-keyring"
)

// keyringService is the OS keyring service namespace the main cmdop CLI uses
// for all of its credentials at rest. Matching it exactly (not inventing a
// separate "cmdop-care" namespace) is what makes the "host-keyring
// passthrough" model work: this binary reads the SAME entries the enrolled
// CLI already wrote, it does not stage its own copy.
const keyringService = "cmdop"

// tokenKey is the per-mode key the main CLI uses for the production relay
// token when no multi-server "active_server" entry is configured (the
// common single-fleet case this artifact targets). See the private repo's
// internal/security/auth/CLAUDE.md "Token model" table — key `token_prod`.
//
// LIMITATION (documented, not silently papered over): a host using cmdop's
// newer per-server multi-fleet token storage (`token_server_<slug>` keys)
// is NOT read by this artifact. That storage layout binds a token to a
// specific server slug chosen at `cmdop login` time, which this artifact
// has no way to discover without depending on cmdop's config package. The
// common case — a single enrolled fleet via `cmdop enroll` — always uses
// the per-mode key below and works correctly.
const tokenKey = "token_prod"

// ErrNotEnrolled is returned when no relay token is found in the keyring —
// the expected state on a host that has not run `cmdop enroll`.
var ErrNotEnrolled = errors.New("cmdop-care: no relay token in the OS keyring — run `cmdop enroll <enrollment-password>` on this host first")

// ErrExpiredToken is returned when a stored token has a non-zero expiry in
// the past. cmdop-care never refreshes or re-mints a token (it has no
// enrollment path); the fix is re-running `cmdop enroll` with the main CLI.
var ErrExpiredToken = errors.New("cmdop-care: relay token expired — re-run `cmdop enroll <enrollment-password>` with the cmdop CLI")

// token is the subset of the private repo's auth.Token JSON shape this
// artifact needs. Decoding via a narrower struct is deliberate: unknown
// fields in the real token blob (workspace_id, user_email, …) are ignored
// rather than round-tripped, so this artifact can never accidentally leak
// them — it only ever reads access_token and expires_at.
type token struct {
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// RelayToken resolves the enrolled host's relay Bearer token from the OS
// keyring. It never prompts, never falls back to an environment variable,
// and never performs any network call.
func RelayToken() (string, error) {
	raw, err := keyring.Get(keyringService, tokenKey)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", ErrNotEnrolled
		}
		return "", fmt.Errorf("cmdop-care: read relay token from OS keyring: %w", err)
	}
	var t token
	if err := json.Unmarshal([]byte(raw), &t); err != nil {
		return "", fmt.Errorf("cmdop-care: decode relay token: %w", err)
	}
	if t.AccessToken == "" {
		return "", ErrNotEnrolled
	}
	if !t.ExpiresAt.IsZero() && time.Now().After(t.ExpiresAt) {
		return "", ErrExpiredToken
	}
	return t.AccessToken, nil
}

// machinePINKeyPrefix matches internal/security/machinepw's
// "machinepw:id:<machineID>" key format exactly (see that package's
// CLAUDE.md), so a PIN the real cmdop CLI has already cached for a machine
// is transparently available here.
const machinePINKeyPrefix = "machinepw:id:"

// ErrNoPINCached is returned when no PIN is stored for the given machine ID.
// care_diagnose treats this as fail-closed: it never prompts for a PIN and
// never sends an empty one hoping the target is unarmed silently — the
// relay call simply proceeds with an empty PIN (matching the private
// contract: an unarmed target ignores it, an armed target denies it), and
// this error lets the tool describe the situation accurately to the caller.
var ErrNoPINCached = errors.New("cmdop-care: no connection PIN cached for this machine")

// MachinePIN reads the cached per-machine connection PIN by immutable
// machine ID, exactly as internal/security/machinepw.Store.Get does. It
// never prompts and never writes.
func MachinePIN(machineID string) (string, error) {
	if machineID == "" {
		return "", ErrNoPINCached
	}
	pin, err := keyring.Get(keyringService, machinePINKeyPrefix+machineID)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", ErrNoPINCached
		}
		return "", fmt.Errorf("cmdop-care: read machine PIN from OS keyring: %w", err)
	}
	if pin == "" {
		return "", ErrNoPINCached
	}
	return pin, nil
}

// Command cmdop-care is a standalone, read-only MCP stdio server exposing 4
// tools — list_machines, care_status, care_diagnose, care_storage_inventory
// — over an already-enrolled cmdop fleet. See README.md for the full
// contract and the security-architecture-2026-07-18.md review this artifact
// implements.
//
// This binary requires the host machine to already be enrolled via the main
// cmdop CLI (`cmdop enroll <enrollment-password>`). It reads that CLI's
// existing OS-keyring-backed relay token and per-machine PIN cache; it does
// NOT implement enrollment, does NOT accept a token via an environment
// variable or mounted secret as a primary path, and holds no credential of
// its own.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/server"
	"github.com/rs/zerolog"

	"github.com/commandoperator/cmdop-care-mcp/internal/credentials"
	"github.com/commandoperator/cmdop-care-mcp/internal/ratelimit"
	"github.com/commandoperator/cmdop-care-mcp/internal/relay"
	"github.com/commandoperator/cmdop-care-mcp/internal/tools"
)

// version is the exact, immutable artifact version. Never "latest" — see
// publication-blueprint-2026-07-18.md. The VERSION file is the single source
// of truth; release/publish.sh stamps this at build time via
// `-ldflags "-X main.version=$(cat VERSION)"` so the running binary always
// reports the version it was actually tagged/pushed as. This default is only
// what a plain `go build`/`make build` (no ldflags) reports locally.
var version = "dev"

// relayAddrEnv optionally overrides the relay dial target (default:
// relay.DefaultAddr, the local embedded relay on loopback). This is a
// TRANSPORT address override only — it is never a credential and carries no
// secret. Advanced self-hosted setups may run the relay on a non-default
// loopback port.
const relayAddrEnv = "CMDOP_CARE_RELAY_ADDR"

// serverInstructions is the "when to reach for cmdop-care" signal handed to
// the MCP client's model.
const serverInstructions = "cmdop-care gives read-only visibility into the user's own enrolled cmdop " +
	"fleet: machine roster, machine-care health, one bounded live diagnostic, and cmdop-owned storage " +
	"usage. It does not run commands, read files, or delegate tasks — for that, the user would need the " +
	"full cmdop CLI's own MCP surface (`cmdop mcp stdio`), which this artifact intentionally does not " +
	"expose. Start with list_machines."

func main() {
	logger := zerolog.New(os.Stderr).With().Timestamp().Str("component", "cmdop-care").Logger()

	// stdio reserves stdout for the MCP protocol; every log line goes to
	// stderr only, and never carries a token, PIN, or full response payload
	// (security review §10) — only tool name / outcome / duration below.
	srv := server.NewMCPServer(
		"cmdop-care",
		version,
		server.WithToolCapabilities(true),
		server.WithInstructions(serverInstructions),
		server.WithLogging(),
	)

	limiter := ratelimit.New(2, 5) // 2 calls/sec steady-state per tool, burst of 5

	dial := func(ctx context.Context) (*relay.Client, error) {
		token, err := credentials.RelayToken()
		if err != nil {
			logger.Warn().Err(err).Msg("no usable relay token in OS keyring")
			return nil, err
		}
		addr := os.Getenv(relayAddrEnv)
		client, err := relay.Dial(addr, token)
		if err != nil {
			logger.Error().Err(err).Msg("relay dial failed")
			return nil, err
		}
		return client, nil
	}

	registry := tools.New(dial, limiter)
	registry.Register(srv)

	logger.Info().Str("version", version).Msg("cmdop-care MCP server starting on stdio")

	if err := server.ServeStdio(srv); err != nil {
		fmt.Fprintf(os.Stderr, "cmdop-care: %v\n", err)
		os.Exit(1)
	}
}

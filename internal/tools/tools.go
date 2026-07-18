// Package tools registers the exactly-4 read-only MCP tools this artifact
// exposes: list_machines, care_status, care_diagnose, care_storage_inventory.
//
// Every tool call is wrapped with an explicit per-call context timeout
// (security review §8 gap — the relay's own HTTP/gRPC client defaults are
// not trusted to bound a call on their own) and gated by a per-tool rate
// limiter (security review §7/§12 defense-in-depth).
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/commandoperator/cmdop-care-mcp/internal/ratelimit"
	"github.com/commandoperator/cmdop-care-mcp/internal/relay"
)

// CallTimeout bounds every single tool call end-to-end, independent of
// whatever timeout (if any) the relay's own transport applies. Recommended
// by the security review §8 ("recommend 15s").
const CallTimeout = 15 * time.Second

// notEnrolledMsg is returned whenever the relay client could not be built —
// almost always because the host has no relay token in its OS keyring yet.
const notEnrolledMsg = "cmdop-care requires an already-enrolled cmdop CLI on this host. " +
	"Run `cmdop enroll <enrollment-password>` with the main cmdop CLI, then retry."

// Registry builds and registers the 4 approved tools against an MCP server.
// dial is injected so tests can substitute a fake relay without a real
// keyring or network dependency; production wires relay.Dial with a token
// resolved from internal/credentials.
type Registry struct {
	dial    func(ctx context.Context) (*relay.Client, error)
	limiter *ratelimit.Limiter
}

// New builds a Registry. dial is called fresh on every tool invocation
// (matching the private product's nil-safe / re-resolved-credential model)
// rather than held open for the process lifetime, so a token that starts
// working mid-session (the user runs `cmdop enroll` while cmdop-care is
// already attached to an MCP client) is picked up on the next call.
func New(dial func(ctx context.Context) (*relay.Client, error), limiter *ratelimit.Limiter) *Registry {
	return &Registry{dial: dial, limiter: limiter}
}

// Register adds all 4 tools to srv.
func (r *Registry) Register(srv *server.MCPServer) {
	for _, def := range r.Definitions() {
		srv.AddTool(def.Tool, def.Handler)
	}
}

// Definition pairs one tool's schema with its guarded handler. Exported so
// tests can exercise handlers directly without a running MCPServer.
type Definition struct {
	Tool    mcp.Tool
	Handler server.ToolHandlerFunc
}

// Definitions returns the exact 4 approved tool definitions, in registration
// order. This is the single source of truth main.go and every test read —
// there is no second hardcoded tool list anywhere in this module.
func (r *Registry) Definitions() []Definition {
	defs := make([]Definition, 0, 4)
	for _, mk := range []func() (mcp.Tool, server.ToolHandlerFunc){
		r.listMachinesTool,
		r.careStatusTool,
		r.careDiagnoseTool,
		r.careStorageInventoryTool,
	} {
		tool, handler := mk()
		defs = append(defs, Definition{Tool: tool, Handler: handler})
	}
	return defs
}

func machineArgSchema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{Type: "object", Properties: map[string]any{
		"machine": map[string]any{"type": "string", "description": "Exact enrolled machine display name from list_machines."},
	}, Required: []string{"machine"}}
}

// --- list_machines ----------------------------------------------------------

func (r *Registry) listMachinesTool() (mcp.Tool, server.ToolHandlerFunc) {
	tool := mcp.Tool{
		Name: "list_machines",
		Description: "List the machines in the user's enrolled cmdop fleet, with each machine's " +
			"online status and OS. Call this first before care_status/care_diagnose/care_storage_inventory " +
			"so you know the exact machine name. Read-only. Does not return machine IDs/UUIDs.",
		InputSchema: mcp.ToolInputSchema{Type: "object", Properties: map[string]any{}},
	}
	return tool, r.withGuards("list_machines", r.handleListMachines)
}

func (r *Registry) handleListMachines(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := r.dial(ctx)
	if err != nil {
		return mcp.NewToolResultText(notEnrolledMsg), nil
	}
	defer client.Close()

	list, err := client.ListMachines(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("list machines failed: %v", err)), nil
	}
	result := mcp.NewToolResultText(renderMachineList(list))
	result.StructuredContent = list
	return result, nil
}

func renderMachineList(list []relay.Machine) string {
	if len(list) == 0 {
		return "No machines in the fleet (none enrolled, or this host is not enrolled)."
	}
	sorted := make([]relay.Machine, len(list))
	copy(sorted, list)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Online != sorted[j].Online {
			return sorted[i].Online
		}
		return strings.ToLower(sorted[i].Host) < strings.ToLower(sorted[j].Host)
	})
	var b strings.Builder
	fmt.Fprintf(&b, "%d machine(s):\n", len(sorted))
	for _, m := range sorted {
		status := "offline"
		if m.Online {
			status = "online"
		}
		osStr := m.OS
		if osStr == "" {
			osStr = "unknown"
		}
		fmt.Fprintf(&b, "- %s [%s, %s]\n", m.Host, status, osStr)
	}
	return b.String()
}

// --- care_status -------------------------------------------------------------

func (r *Registry) careStatusTool() (mcp.Tool, server.ToolHandlerFunc) {
	tool := mcp.Tool{
		Name: "care_status",
		Description: "Read durable Machine Care facts, findings, coverage, and last-known freshness " +
			"for one enrolled machine. Read-only; can work while the target is offline (it reads a stored " +
			"projection, never a live round-trip).",
		InputSchema: machineArgSchema(),
	}
	return tool, r.withGuards("care_status", r.handleCareStatus)
}

func (r *Registry) handleCareStatus(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	machine, failure := r.requireMachine(req)
	if failure != nil {
		return failure, nil
	}
	client, err := r.dial(ctx)
	if err != nil {
		return mcp.NewToolResultText(notEnrolledMsg), nil
	}
	defer client.Close()

	value, err := client.CareStatus(ctx, machine)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("care status for %q failed: %v", machine, err)), nil
	}
	return structuredResult(value)
}

// --- care_diagnose ------------------------------------------------------------

func (r *Registry) careDiagnoseTool() (mcp.Tool, server.ToolHandlerFunc) {
	tool := mcp.Tool{
		Name: "care_diagnose",
		Description: "Run one bounded, typed process/startup diagnostic on a live enrolled machine. " +
			"Use this to explain why a machine is slow. It never executes shell, accepts no PID or command " +
			"argument, and performs no mutation. " +
			"DISCLOSURE 1: if this machine requires a connection PIN, it is forwarded automatically from " +
			"this host's local OS-keyring PIN cache — you cannot and do not need to supply one; if no PIN " +
			"is cached for an armed machine, the call fails closed with a permission error. " +
			"DISCLOSURE 2: the response includes raw OS process IDs (PIDs) and process names from the " +
			"target machine — that data will reach whatever LLM/MCP client is calling this tool.",
		InputSchema: machineArgSchema(),
	}
	return tool, r.withGuards("care_diagnose", r.handleCareDiagnose)
}

func (r *Registry) handleCareDiagnose(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	machine, failure := r.requireMachine(req)
	if failure != nil {
		return failure, nil
	}
	client, err := r.dial(ctx)
	if err != nil {
		return mcp.NewToolResultText(notEnrolledMsg), nil
	}
	defer client.Close()

	value, err := client.CareDiagnose(ctx, machine)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("care diagnostic for %q failed: %v", machine, err)), nil
	}
	return structuredResult(value)
}

// --- care_storage_inventory ---------------------------------------------------

func (r *Registry) careStorageInventoryTool() (mcp.Tool, server.ToolHandlerFunc) {
	tool := mcp.Tool{
		Name: "care_storage_inventory",
		Description: "Read the latest validated cmdop-owned storage inventory for one enrolled machine. " +
			"Use this to explain cmdop-related disk usage. Read-only; accepts no path argument and deletes " +
			"nothing.",
		InputSchema: machineArgSchema(),
	}
	return tool, r.withGuards("care_storage_inventory", r.handleCareStorageInventory)
}

func (r *Registry) handleCareStorageInventory(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	machine, failure := r.requireMachine(req)
	if failure != nil {
		return failure, nil
	}
	client, err := r.dial(ctx)
	if err != nil {
		return mcp.NewToolResultText(notEnrolledMsg), nil
	}
	defer client.Close()

	value, err := client.CareStorageInventory(ctx, machine)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("care storage inventory for %q failed: %v", machine, err)), nil
	}
	return structuredResult(value)
}

// --- shared helpers -----------------------------------------------------------

// requireMachine extracts and validates the "machine" argument. The value is
// used purely as an opaque server-side lookup key — never interpolated into
// a path, shell command, or query string anywhere in this artifact (see
// internal/relay, which only ever passes it as a protobuf string field).
func (r *Registry) requireMachine(req mcp.CallToolRequest) (string, *mcp.CallToolResult) {
	machine := strings.TrimSpace(getStringArg(req.Params.Arguments, "machine"))
	if machine == "" {
		return "", mcp.NewToolResultError("machine is required")
	}
	return machine, nil
}

func getStringArg(args any, key string) string {
	m, ok := args.(map[string]any)
	if !ok {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

func structuredResult(value any) (*mcp.CallToolResult, error) {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	result := mcp.NewToolResultText(string(payload))
	result.StructuredContent = value
	return result, nil
}

// withGuards wraps a handler with: (1) the per-tool rate limiter, checked
// first so a denied call never even reaches the network, and (2) an
// explicit CallTimeout bound on ctx, independent of any timeout the relay
// transport itself applies.
func (r *Registry) withGuards(name string, next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if r.limiter != nil && !r.limiter.Allow(name) {
			return mcp.NewToolResultError(fmt.Sprintf("rate limit exceeded for %q — slow down and retry shortly", name)), nil
		}
		callCtx, cancel := context.WithTimeout(ctx, CallTimeout)
		defer cancel()
		return next(callCtx, req)
	}
}

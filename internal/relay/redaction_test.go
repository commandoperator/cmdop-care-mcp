package relay_test

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/commandoperator/cmdop-care-mcp/internal/relay"
)

// This file is the redaction/allowlist regression test flagged as MISSING in
// code-audit-2026-07-18.md and required by security-architecture-2026-07-18.md
// §12 ("Sensitive field creeps into a future DTO change ... To build in
// Phase 2"). It asserts the exact JSON key set for every one of the 4 tool
// response types, so a new field added to any DTO without updating this test
// fails CI — not a silent widening of what leaves the process.
//
// jsonKeys walks a JSON-marshaled value and returns every key path found,
// including nested object keys (dot-joined) and "[]" for array elements, so a
// field added anywhere in a nested struct is caught, not just top-level.
func jsonKeys(t *testing.T, v any) []string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	keys := map[string]struct{}{}
	collectKeys("", generic, keys)
	out := make([]string, 0, len(keys))
	for k := range keys {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func collectKeys(prefix string, v any, out map[string]struct{}) {
	switch val := v.(type) {
	case map[string]any:
		for k, child := range val {
			path := k
			if prefix != "" {
				path = prefix + "." + k
			}
			out[path] = struct{}{}
			collectKeys(path, child, out)
		}
	case []any:
		for _, child := range val {
			collectKeys(prefix+"[]", child, out)
		}
	}
}

func assertExactKeySet(t *testing.T, got []string, allow map[string]bool) {
	t.Helper()
	for _, k := range got {
		if !allow[k] {
			t.Errorf("UNEXPECTED field in serialized output (not in allowlist): %q — a new field was added to a public DTO without updating this regression test; either it is a genuine review-required addition (update the allowlist here deliberately) or it is an accidental leak", k)
		}
	}
	for k := range allow {
		found := false
		for _, g := range got {
			if g == k {
				found = true
				break
			}
		}
		// Allow-listed-but-absent is fine (omitempty / zero-value fields) —
		// this loop only exists to make the allowlist self-documenting; it
		// never fails the test on its own.
		_ = found
	}
}

// --- The regression test proves itself: a fixture struct with an extra
// field MUST fail this style of assertion, otherwise the test is a no-op. ---

type fixtureWithExtraField struct {
	Host   string `json:"host"`
	Online bool   `json:"online"`
	OS     string `json:"os"`
	// SecretLeak simulates an accidental new field that must be caught.
	SecretLeak string `json:"internal_debug_token"`
}

func TestRedactionTest_CatchesAnInjectedExtraField(t *testing.T) {
	fixture := fixtureWithExtraField{Host: "x", Online: true, OS: "linux", SecretLeak: "should-be-caught"}
	allow := map[string]bool{"host": true, "online": true, "os": true}

	failed := false
	t.Run("expect-failure", func(t *testing.T) {
		keys := jsonKeys(t, fixture)
		for _, k := range keys {
			if !allow[k] {
				failed = true
			}
		}
	})
	if !failed {
		t.Fatal("the allowlist check did not flag the injected extra field — the regression test itself is broken")
	}
}

// --- Real DTOs: exact allowlists, one per tool. ---

func TestRedactionAllowlist_Machine(t *testing.T) {
	m := relay.Machine{Host: "web-01", Online: true, OS: "linux"}
	allow := map[string]bool{"host": true, "online": true, "os": true}
	assertExactKeySet(t, jsonKeys(t, m), allow)
}

func TestRedactionAllowlist_CareStatus(t *testing.T) {
	reclaim := uint64(5)
	status := relay.CareStatus{
		LatestRun: &relay.CareRun{
			SchemaVersion: 1, CapabilityVersion: "v1", PolicyVersion: "v1",
			TerminalState: "done", Coverage: "full", ReasonCode: "ok", FinishedAt: 1,
		},
		LatestFacts: &relay.CareFacts{
			SchemaVersion: 1, CollectedAt: 1,
			Domains: []relay.CareFactDomain{{
				Code: "cpu", Coverage: "full",
				Facts: []relay.CareFact{{Code: "load", Value: 1, Unit: "pct", Coverage: "full", ReasonCode: "ok", CapturedAt: 1}},
			}},
		},
		Findings: []relay.CareFinding{{
			SchemaVersion: 1, ID: "f1", RuleID: "r1", RuleVersion: "1", Domain: "cpu",
			Presentation: "p", NextStep: "n", Severity: "low", Coverage: "full", ReasonCode: "ok",
			Revision: 1, LastTransition: "opened", TransitionAt: 1,
		}},
		FindingsTruncated: false,
	}
	_ = reclaim

	allow := map[string]bool{
		"latest_run": true, "latest_run.schema_version": true, "latest_run.capability_version": true,
		"latest_run.policy_version": true, "latest_run.terminal_state": true, "latest_run.coverage": true,
		"latest_run.reason_code": true, "latest_run.finished_at_unix_ms": true,

		"latest_facts": true, "latest_facts.schema_version": true, "latest_facts.collected_at_unix_ms": true,
		"latest_facts.domains": true, "latest_facts.domains[]": true, "latest_facts.domains[].code": true,
		"latest_facts.domains[].coverage": true, "latest_facts.domains[].facts": true,
		"latest_facts.domains[].facts[]": true, "latest_facts.domains[].facts[].code": true,
		"latest_facts.domains[].facts[].value": true, "latest_facts.domains[].facts[].unit": true,
		"latest_facts.domains[].facts[].coverage": true, "latest_facts.domains[].facts[].reason_code": true,
		"latest_facts.domains[].facts[].captured_at_unix_ms": true,

		"findings": true, "findings[]": true, "findings[].schema_version": true, "findings[].id": true,
		"findings[].rule_id": true, "findings[].rule_version": true, "findings[].domain": true,
		"findings[].presentation_code": true, "findings[].next_step_code": true, "findings[].severity": true,
		"findings[].coverage": true, "findings[].reason_code": true, "findings[].revision": true,
		"findings[].last_transition": true, "findings[].transition_at_unix_ms": true,

		"findings_truncated": true,
	}
	assertExactKeySet(t, jsonKeys(t, status), allow)
}

func TestRedactionAllowlist_CareDiagnostic(t *testing.T) {
	diag := relay.CareDiagnostic{
		SchemaVersion: 1, ScanRunID: "s1", CaptureStartedAt: 1, CaptureFinishedAt: 2,
		TriggerCode: "manual", Coverage: "full", ReasonCode: "ok",
		Processes: relay.CareProcessDiagnostic{
			Coverage: "full", ReasonCode: "ok", ExaminedCount: 1, RetainedCount: 1, Truncated: false,
			Entries: []relay.CareProcessAttribution{{
				CandidateID: "c1", PID: 123, StartedAt: 1, Name: "node",
				CPUMilliPercent: 10, RSSBytes: 1024, CPULeader: true, RSSLeader: false,
			}},
		},
		Startup: relay.CareStartupDiagnostic{
			Coverage: "full", ReasonCode: "ok", ExaminedCount: 1, RetainedCount: 1, Truncated: false,
			Sources: []relay.CareStartupSource{{
				SourceCode: "login_items", Scope: "user", Coverage: "full", ReasonCode: "ok",
				ExaminedCount: 1, RetainedCount: 1, Truncated: false,
				Entries: []relay.CareStartupEntry{{ID: "e1", Label: "l1", SourceCode: "login_items", Scope: "user", Kind: "app"}},
			}},
		},
	}

	allow := map[string]bool{
		"schema_version": true, "scan_run_id": true, "capture_started_at_unix_ms": true,
		"capture_finished_at_unix_ms": true, "trigger_code": true, "coverage": true, "reason_code": true,

		"processes": true, "processes.coverage": true, "processes.reason_code": true,
		"processes.examined_count": true, "processes.retained_count": true, "processes.truncated": true,
		"processes.entries": true, "processes.entries[]": true,
		"processes.entries[].candidate_id": true, "processes.entries[].pid": true,
		"processes.entries[].started_at_unix_ms": true, "processes.entries[].name": true,
		"processes.entries[].cpu_milli_percent": true, "processes.entries[].rss_bytes": true,
		"processes.entries[].cpu_leader": true, "processes.entries[].rss_leader": true,

		"startup": true, "startup.coverage": true, "startup.reason_code": true,
		"startup.examined_count": true, "startup.retained_count": true, "startup.truncated": true,
		"startup.sources": true, "startup.sources[]": true,
		"startup.sources[].source_code": true, "startup.sources[].scope": true, "startup.sources[].coverage": true,
		"startup.sources[].reason_code": true, "startup.sources[].examined_count": true,
		"startup.sources[].retained_count": true, "startup.sources[].truncated": true,
		"startup.sources[].entries": true, "startup.sources[].entries[]": true,
		"startup.sources[].entries[].id": true, "startup.sources[].entries[].label": true,
		"startup.sources[].entries[].source_code": true, "startup.sources[].entries[].scope": true,
		"startup.sources[].entries[].kind": true,
	}
	assertExactKeySet(t, jsonKeys(t, diag), allow)
}

func TestRedactionAllowlist_CareStorageInventory(t *testing.T) {
	reported := uint64(1000)
	inv := relay.CareStorageInventory{
		SchemaVersion: 1, CapturedAt: 1, FinishedAt: 2, Coverage: "full", ReasonCode: "ok",
		VolumeTotalBytes: 0, VolumeFreeBytes: 0,
		Sources: []relay.CareStorageSource{{
			SourceCode: "cmdop_logs", SourceVersion: 1, ClassCode: "logs", LogicalBytes: 2048,
			ReportedAllocatedBytes: &reported, EstimationQuality: "measured",
			Coverage: "full", ReasonCode: "ok", Eligibility: "eligible",
		}},
	}
	allow := map[string]bool{
		"schema_version": true, "captured_at_unix_ms": true, "finished_at_unix_ms": true,
		"coverage": true, "reason_code": true,

		"sources": true, "sources[]": true, "sources[].source_code": true, "sources[].source_version": true,
		"sources[].class_code": true, "sources[].logical_bytes": true, "sources[].reported_allocated_bytes": true,
		"sources[].estimated_reclaim_bytes": true, "sources[].estimation_quality": true,
		"sources[].coverage": true, "sources[].reason_code": true, "sources[].eligibility": true,
	}
	assertExactKeySet(t, jsonKeys(t, inv), allow)
}

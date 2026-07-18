package relay

import "github.com/commandoperator/cmdop-care-mcp/internal/carepb"

// The types below are a public, redacted read model — the exact allowlist
// the redaction regression test (internal/relay/redaction_test.go) pins.
// Every field is a bounded, closed-vocabulary or numeric fact; none is a raw
// path, command, secret, token, environment variable, or another tenant's
// identifier. The one intentional, disclosed exception is
// CareProcessAttribution.PID/Name — see the package doc and README.

type CareStatus struct {
	LatestRun         *CareRun      `json:"latest_run,omitempty"`
	LatestFacts       *CareFacts    `json:"latest_facts,omitempty"`
	Findings          []CareFinding `json:"findings"`
	FindingsTruncated bool          `json:"findings_truncated"`
}

type CareRun struct {
	SchemaVersion     uint32 `json:"schema_version"`
	CapabilityVersion string `json:"capability_version"`
	PolicyVersion     string `json:"policy_version"`
	TerminalState     string `json:"terminal_state"`
	Coverage          string `json:"coverage"`
	ReasonCode        string `json:"reason_code,omitempty"`
	FinishedAt        int64  `json:"finished_at_unix_ms"`
}

type CareFinding struct {
	SchemaVersion  uint32 `json:"schema_version"`
	ID             string `json:"id"`
	RuleID         string `json:"rule_id"`
	RuleVersion    string `json:"rule_version"`
	Domain         string `json:"domain,omitempty"`
	Presentation   string `json:"presentation_code,omitempty"`
	NextStep       string `json:"next_step_code,omitempty"`
	Severity       string `json:"severity"`
	Coverage       string `json:"coverage"`
	ReasonCode     string `json:"reason_code,omitempty"`
	Revision       uint64 `json:"revision"`
	LastTransition string `json:"last_transition,omitempty"`
	TransitionAt   int64  `json:"transition_at_unix_ms,omitempty"`
}

type CareFacts struct {
	SchemaVersion uint32           `json:"schema_version"`
	CollectedAt   int64            `json:"collected_at_unix_ms"`
	Domains       []CareFactDomain `json:"domains"`
}

type CareFactDomain struct {
	Code     string     `json:"code"`
	Coverage string     `json:"coverage"`
	Facts    []CareFact `json:"facts"`
}

type CareFact struct {
	Code       string `json:"code"`
	Value      int64  `json:"value"`
	Unit       string `json:"unit"`
	Coverage   string `json:"coverage"`
	ReasonCode string `json:"reason_code,omitempty"`
	CapturedAt int64  `json:"captured_at_unix_ms"`
}

type CareDiagnostic struct {
	SchemaVersion     uint32                `json:"schema_version"`
	ScanRunID         string                `json:"scan_run_id"`
	CaptureStartedAt  int64                 `json:"capture_started_at_unix_ms"`
	CaptureFinishedAt int64                 `json:"capture_finished_at_unix_ms"`
	TriggerCode       string                `json:"trigger_code"`
	Coverage          string                `json:"coverage"`
	ReasonCode        string                `json:"reason_code,omitempty"`
	Processes         CareProcessDiagnostic `json:"processes"`
	Startup           CareStartupDiagnostic `json:"startup"`
}

type CareProcessDiagnostic struct {
	Coverage      string                   `json:"coverage"`
	ReasonCode    string                   `json:"reason_code,omitempty"`
	ExaminedCount uint32                   `json:"examined_count"`
	RetainedCount uint32                   `json:"retained_count"`
	Truncated     bool                     `json:"truncated"`
	Entries       []CareProcessAttribution `json:"entries"`
}

// CareProcessAttribution carries raw PID and process Name — the disclosed
// exception documented in the package doc, the README, and the
// care_diagnose tool description.
type CareProcessAttribution struct {
	CandidateID     string `json:"candidate_id"`
	PID             int32  `json:"pid"`
	StartedAt       int64  `json:"started_at_unix_ms"`
	Name            string `json:"name"`
	CPUMilliPercent int64  `json:"cpu_milli_percent"`
	RSSBytes        uint64 `json:"rss_bytes"`
	CPULeader       bool   `json:"cpu_leader"`
	RSSLeader       bool   `json:"rss_leader"`
}

type CareStartupDiagnostic struct {
	Coverage      string              `json:"coverage"`
	ReasonCode    string              `json:"reason_code,omitempty"`
	ExaminedCount uint32              `json:"examined_count"`
	RetainedCount uint32              `json:"retained_count"`
	Truncated     bool                `json:"truncated"`
	Sources       []CareStartupSource `json:"sources"`
}

type CareStartupSource struct {
	SourceCode    string             `json:"source_code"`
	Scope         string             `json:"scope"`
	Coverage      string             `json:"coverage"`
	ReasonCode    string             `json:"reason_code,omitempty"`
	ExaminedCount uint32             `json:"examined_count"`
	RetainedCount uint32             `json:"retained_count"`
	Truncated     bool               `json:"truncated"`
	Entries       []CareStartupEntry `json:"entries"`
}

type CareStartupEntry struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	SourceCode string `json:"source_code"`
	Scope      string `json:"scope"`
	Kind       string `json:"kind"`
}

type CareStorageInventory struct {
	SchemaVersion    uint32              `json:"schema_version"`
	CapturedAt       int64               `json:"captured_at_unix_ms"`
	FinishedAt       int64               `json:"finished_at_unix_ms"`
	Coverage         string              `json:"coverage"`
	ReasonCode       string              `json:"reason_code,omitempty"`
	VolumeTotalBytes uint64              `json:"volume_total_bytes,omitempty"`
	VolumeFreeBytes  uint64              `json:"volume_free_bytes,omitempty"`
	Sources          []CareStorageSource `json:"sources"`
}

type CareStorageSource struct {
	SourceCode             string  `json:"source_code"`
	SourceVersion          uint32  `json:"source_version"`
	ClassCode              string  `json:"class_code"`
	LogicalBytes           uint64  `json:"logical_bytes"`
	ReportedAllocatedBytes *uint64 `json:"reported_allocated_bytes,omitempty"`
	EstimatedReclaimBytes  *uint64 `json:"estimated_reclaim_bytes,omitempty"`
	EstimationQuality      string  `json:"estimation_quality"`
	Coverage               string  `json:"coverage"`
	ReasonCode             string  `json:"reason_code,omitempty"`
	Eligibility            string  `json:"eligibility"`
}

func projectCareStatus(snapshot *carepb.CareSnapshot) CareStatus {
	out := CareStatus{Findings: make([]CareFinding, 0, len(snapshot.GetFindings()))}
	if scan := snapshot.GetLatestScan(); scan != nil {
		out.LatestRun = &CareRun{
			SchemaVersion:     snapshot.GetSchemaVersion(),
			CapabilityVersion: scan.GetCapabilityVersion(),
			PolicyVersion:     scan.GetPolicyVersion(),
			TerminalState:     scan.GetTerminalState(),
			Coverage:          scan.GetCoverage(),
			ReasonCode:        scan.GetReasonCode(),
			FinishedAt:        scan.GetFinishedAtUnixMs(),
		}
	}
	if facts := snapshot.GetLatestFacts(); facts != nil {
		mapped := CareFacts{
			SchemaVersion: facts.GetSchemaVersion(),
			CollectedAt:   facts.GetCollectedAtUnixMs(),
			Domains:       make([]CareFactDomain, 0, len(facts.GetDomains())),
		}
		for _, group := range facts.GetDomains() {
			domain := CareFactDomain{Code: group.GetCode(), Coverage: group.GetCoverage(), Facts: make([]CareFact, 0, len(group.GetFacts()))}
			for _, fact := range group.GetFacts() {
				domain.Facts = append(domain.Facts, CareFact{
					Code: fact.GetCode(), Value: fact.GetValue(), Unit: fact.GetUnit(),
					Coverage: fact.GetCoverage(), ReasonCode: fact.GetReasonCode(), CapturedAt: fact.GetCapturedAtUnixMs(),
				})
			}
			mapped.Domains = append(mapped.Domains, domain)
		}
		out.LatestFacts = &mapped
	}
	for _, finding := range snapshot.GetFindings() {
		out.Findings = append(out.Findings, CareFinding{
			SchemaVersion: snapshot.GetSchemaVersion(), ID: finding.GetProjectionId(),
			RuleID: finding.GetRuleCode(), RuleVersion: finding.GetRuleVersion(), Domain: finding.GetDomain(),
			Presentation: finding.GetPresentationCode(), NextStep: finding.GetNextStepCode(), Severity: finding.GetSeverity(),
			Coverage: finding.GetCoverage(), ReasonCode: finding.GetReasonCode(), Revision: finding.GetRevision(),
			LastTransition: finding.GetLastTransition(), TransitionAt: finding.GetTransitionAtUnixMs(),
		})
	}
	return out
}

func projectCareStorage(snapshot *carepb.CareStorageSnapshot) CareStorageInventory {
	out := CareStorageInventory{
		SchemaVersion: snapshot.GetSchemaVersion(), CapturedAt: snapshot.GetCapturedAtUnixMs(),
		FinishedAt: snapshot.GetFinishedAtUnixMs(), Coverage: snapshot.GetCoverage(), ReasonCode: snapshot.GetReasonCode(),
		Sources: make([]CareStorageSource, 0, len(snapshot.GetSources())),
	}
	for _, source := range snapshot.GetSources() {
		mapped := CareStorageSource{
			SourceCode: source.GetSourceCode(), SourceVersion: source.GetSourceVersion(),
			ClassCode: source.GetClass(), LogicalBytes: source.GetLogicalBytes(), EstimationQuality: source.GetEstimationQuality(),
			Coverage: source.GetCoverage(), ReasonCode: source.GetReasonCode(), Eligibility: source.GetEligibility(),
		}
		if source.ReportedAllocatedBytes != nil {
			v := source.GetReportedAllocatedBytes()
			mapped.ReportedAllocatedBytes = &v
		}
		if source.EstimatedReclaimBytes != nil {
			v := source.GetEstimatedReclaimBytes()
			mapped.EstimatedReclaimBytes = &v
		}
		out.Sources = append(out.Sources, mapped)
	}
	return out
}

func projectCareDiagnostic(snapshot *carepb.CareDiagnostic) CareDiagnostic {
	out := CareDiagnostic{
		SchemaVersion: snapshot.GetSchemaVersion(), ScanRunID: snapshot.GetScanRunId(),
		CaptureStartedAt: snapshot.GetCaptureStartedAtUnixMs(), CaptureFinishedAt: snapshot.GetCaptureFinishedAtUnixMs(),
		TriggerCode: snapshot.GetTriggerCode(), Coverage: snapshot.GetCoverage(), ReasonCode: snapshot.GetReasonCode(),
		Processes: CareProcessDiagnostic{
			Coverage: snapshot.GetProcesses().GetCoverage(), ReasonCode: snapshot.GetProcesses().GetReasonCode(),
			ExaminedCount: snapshot.GetProcesses().GetExaminedCount(), RetainedCount: snapshot.GetProcesses().GetRetainedCount(),
			Truncated: snapshot.GetProcesses().GetTruncated(), Entries: make([]CareProcessAttribution, 0, len(snapshot.GetProcesses().GetEntries())),
		},
		Startup: CareStartupDiagnostic{
			Coverage: snapshot.GetStartup().GetCoverage(), ReasonCode: snapshot.GetStartup().GetReasonCode(),
			ExaminedCount: snapshot.GetStartup().GetExaminedCount(), RetainedCount: snapshot.GetStartup().GetRetainedCount(),
			Truncated: snapshot.GetStartup().GetTruncated(), Sources: make([]CareStartupSource, 0, len(snapshot.GetStartup().GetSources())),
		},
	}
	for _, process := range snapshot.GetProcesses().GetEntries() {
		out.Processes.Entries = append(out.Processes.Entries, CareProcessAttribution{
			CandidateID: process.GetCandidateId(),
			PID:         process.GetPid(), StartedAt: process.GetStartedAtUnixMs(), Name: process.GetName(),
			CPUMilliPercent: process.GetCpuMilliPercent(), RSSBytes: process.GetRssBytes(),
			CPULeader: process.GetCpuLeader(), RSSLeader: process.GetRssLeader(),
		})
	}
	for _, source := range snapshot.GetStartup().GetSources() {
		mapped := CareStartupSource{
			SourceCode: source.GetSourceCode(), Scope: source.GetScope(), Coverage: source.GetCoverage(),
			ReasonCode: source.GetReasonCode(), ExaminedCount: source.GetExaminedCount(), RetainedCount: source.GetRetainedCount(),
			Truncated: source.GetTruncated(), Entries: make([]CareStartupEntry, 0, len(source.GetEntries())),
		}
		for _, entry := range source.GetEntries() {
			mapped.Entries = append(mapped.Entries, CareStartupEntry{
				ID: entry.GetId(), Label: entry.GetLabel(),
				SourceCode: entry.GetSourceCode(), Scope: entry.GetScope(), Kind: entry.GetKind(),
			})
		}
		out.Startup.Sources = append(out.Startup.Sources, mapped)
	}
	return out
}

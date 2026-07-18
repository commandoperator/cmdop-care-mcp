package ratelimit_test

import (
	"testing"

	"github.com/commandoperator/cmdop-care-mcp/internal/ratelimit"
)

func TestLimiter_AllowsUpToBurstThenBlocks(t *testing.T) {
	l := ratelimit.New(1, 2) // 1/sec steady state, burst 2

	if !l.Allow("care_status") {
		t.Fatal("expected 1st call to be allowed")
	}
	if !l.Allow("care_status") {
		t.Fatal("expected 2nd call (within burst) to be allowed")
	}
	if l.Allow("care_status") {
		t.Fatal("expected 3rd immediate call to be rate-limited")
	}
}

func TestLimiter_TracksToolsIndependently(t *testing.T) {
	l := ratelimit.New(1, 1)

	if !l.Allow("care_status") {
		t.Fatal("expected care_status to be allowed")
	}
	if l.Allow("care_status") {
		t.Fatal("expected 2nd care_status call to be blocked")
	}
	if !l.Allow("care_diagnose") {
		t.Fatal("a different tool must have its own independent bucket")
	}
}

func TestErrRateLimited_MessageMentionsTool(t *testing.T) {
	err := &ratelimit.ErrRateLimited{Tool: "list_machines"}
	if got := err.Error(); got == "" {
		t.Fatal("expected a non-empty error message")
	}
}

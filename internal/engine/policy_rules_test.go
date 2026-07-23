package engine

import (
	"strings"
	"testing"
)

func TestPolicyCreateCanNeverBeAutoAllowed(t *testing.T) {
	cfg := map[string]any{"field": "name", "pattern": ".*"}
	for _, pattern := range []string{"aps.policy.create", "aps.policy.*", "aps.*", "*"} {
		err := ValidatePolicySpec("sneaky", pattern, "regex", cfg, VerdictAllow)
		if err == nil || !strings.Contains(err.Error(), "never be auto-allowed") {
			t.Fatalf("pattern %q with effect allow must be rejected, got %v", pattern, err)
		}
		// The same patterns are fine with gate/deny effects.
		if err := ValidatePolicySpec("gate", pattern, "regex", cfg, VerdictRequireApproval); err != nil {
			t.Fatalf("pattern %q with require_approval should pass: %v", pattern, err)
		}
	}
	// Unrelated allows are unaffected.
	if err := ValidatePolicySpec("reads", "db.query", "regex", map[string]any{"field": "sql", "pattern": "^SELECT"}, VerdictAllow); err != nil {
		t.Fatalf("normal allow should pass: %v", err)
	}
}

func TestValidatePatternGrammar(t *testing.T) {
	bad := []string{"", "db.*.write", "*.query", "db*", ".*"}
	for _, p := range bad {
		if err := ValidatePolicySpec("x", p, "exact", map[string]any{"field": "f", "value": "v"}, VerdictDeny); err == nil {
			t.Fatalf("pattern %q should be rejected", p)
		}
	}
	good := []string{"*", "db.query", "db.*", "aps.policy.create"}
	for _, p := range good {
		if err := ValidatePolicySpec("x", p, "exact", map[string]any{"field": "f", "value": "v"}, VerdictDeny); err != nil {
			t.Fatalf("pattern %q should pass: %v", p, err)
		}
	}
}

func TestValidateMatcherConfig(t *testing.T) {
	if err := ValidatePolicySpec("x", "db.query", "regex", map[string]any{"field": "sql", "pattern": "(unclosed"}, VerdictDeny); err == nil {
		t.Fatal("broken regex must be rejected at creation")
	}
	if err := ValidatePolicySpec("x", "db.query", "llm", map[string]any{"field": "sql"}, VerdictDeny); err == nil {
		t.Fatal("llm matcher is reserved, not accepted yet")
	}
	if err := ValidatePolicySpec("x", "db.query", "exact", map[string]any{"value": "v"}, VerdictDeny); err == nil {
		t.Fatal("missing field must be rejected")
	}
}

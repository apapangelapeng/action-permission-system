package engine

import (
	"reflect"
	"testing"
)

func regexPolicy(id, effect, field, pattern string, priority int) Policy {
	return Policy{
		ID: id, Effect: effect, MatcherType: "regex", Priority: priority,
		MatcherConfig: map[string]any{"field": field, "pattern": pattern},
	}
}

func TestCandidateKeys(t *testing.T) {
	got := CandidateKeys("db.admin.drop")
	want := []string{"db.admin.drop", "db.admin.*", "db.*", "*"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := CandidateKeys("deploy"); !reflect.DeepEqual(got, []string{"deploy", "*"}) {
		t.Fatalf("single segment: got %v", got)
	}
}

func TestNoMatchFailsClosed(t *testing.T) {
	res := Evaluate(nil, map[string]any{"sql": "SELECT 1"})
	if res.Verdict != VerdictRequireApproval || res.MatchedPolicyID != "" {
		t.Fatalf("got %+v", res)
	}
}

func TestRequireApprovalBeatsAllow(t *testing.T) {
	pols := []Policy{
		regexPolicy("allow-reads", VerdictAllow, "sql", `(?i)^\s*SELECT`, 100),
		regexPolicy("gate-payments", VerdictRequireApproval, "sql", `(?i)payments`, 50),
	}
	res := Evaluate(pols, map[string]any{"sql": "SELECT * FROM payments"})
	if res.Verdict != VerdictRequireApproval || res.MatchedPolicyID != "gate-payments" {
		t.Fatalf("got %+v", res)
	}
}

func TestDenyWinsRegardlessOfPriority(t *testing.T) {
	pols := []Policy{
		regexPolicy("allow-all-db", VerdictAllow, "sql", `.`, 1),
		regexPolicy("no-drop", VerdictDeny, "sql", `(?i)\bDROP\b`, 999),
	}
	res := Evaluate(pols, map[string]any{"sql": "DROP TABLE users"})
	if res.Verdict != VerdictDeny || res.MatchedPolicyID != "no-drop" {
		t.Fatalf("got %+v", res)
	}
}

func TestAllowWhenOnlyAllowFires(t *testing.T) {
	pols := []Policy{
		regexPolicy("allow-reads", VerdictAllow, "sql", `(?i)^\s*SELECT`, 100),
		regexPolicy("no-drop", VerdictDeny, "sql", `(?i)\bDROP\b`, 10),
	}
	res := Evaluate(pols, map[string]any{"sql": "SELECT id FROM orders"})
	if res.Verdict != VerdictAllow || res.MatchedPolicyID != "allow-reads" {
		t.Fatalf("got %+v", res)
	}
}

func TestBrokenPolicyBlocksAllow(t *testing.T) {
	pols := []Policy{
		regexPolicy("allow-reads", VerdictAllow, "sql", `(?i)^\s*SELECT`, 100),
		regexPolicy("broken", VerdictDeny, "sql", `(unclosed`, 10),
	}
	res := Evaluate(pols, map[string]any{"sql": "SELECT 1"})
	if res.Verdict != VerdictRequireApproval {
		t.Fatalf("a broken gate must fail closed, got %+v", res)
	}
	if len(res.MatcherErrors) != 1 {
		t.Fatalf("expected 1 matcher error, got %v", res.MatcherErrors)
	}
}

func TestExactMatcher(t *testing.T) {
	pols := []Policy{{
		ID: "status-page", Effect: VerdictAllow, MatcherType: "exact", Priority: 100,
		MatcherConfig: map[string]any{"field": "host", "value": "status.internal.example.com"},
	}}
	if res := Evaluate(pols, map[string]any{"host": "status.internal.example.com"}); res.Verdict != VerdictAllow {
		t.Fatalf("got %+v", res)
	}
	if res := Evaluate(pols, map[string]any{"host": "api.stripe.com"}); res.Verdict != VerdictRequireApproval {
		t.Fatalf("got %+v", res)
	}
}

func scopedPolicy(id, effect, field, pattern string, priority int, botID string) Policy {
	p := regexPolicy(id, effect, field, pattern, priority)
	p.BotID = &botID
	return p
}

func TestScopedAllowPiercesGlobalDeny(t *testing.T) {
	pols := []Policy{
		regexPolicy("global-no-secrets", VerdictDeny, "path", `.`, 10),
		scopedPolicy("edge-bot-secrets", VerdictAllow, "path", `^prod/`, 100, "bot_edge"),
	}
	res := Evaluate(pols, map[string]any{"path": "prod/db-password"})
	if res.Verdict != VerdictAllow || res.MatchedPolicyID != "edge-bot-secrets" {
		t.Fatalf("scoped allow must pierce global deny, got %+v", res)
	}
	if !res.OverrodeGlobal {
		t.Fatalf("override should be flagged, got %+v", res)
	}
}

func TestScopedDenyBeatsGlobalAllow(t *testing.T) {
	pols := []Policy{
		regexPolicy("global-exports-ok", VerdictAllow, "table", `.`, 10),
		scopedPolicy("edge-no-exports", VerdictDeny, "table", `.`, 100, "bot_edge"),
	}
	res := Evaluate(pols, map[string]any{"table": "orders"})
	if res.Verdict != VerdictDeny || res.MatchedPolicyID != "edge-no-exports" || !res.OverrodeGlobal {
		t.Fatalf("scoped deny must beat global allow, got %+v", res)
	}
}

func TestGlobalDecidesWhenNoScopedRuleFires(t *testing.T) {
	pols := []Policy{
		regexPolicy("global-no-drop", VerdictDeny, "sql", `DROP`, 10),
		scopedPolicy("scoped-selects", VerdictAllow, "sql", `^SELECT`, 100, "bot_edge"),
	}
	res := Evaluate(pols, map[string]any{"sql": "DROP TABLE users"})
	if res.Verdict != VerdictDeny || res.MatchedPolicyID != "global-no-drop" || res.OverrodeGlobal {
		t.Fatalf("global deny should stand when no scoped rule fires, got %+v", res)
	}
}

func TestDenyStillWinsWithinScopedTier(t *testing.T) {
	pols := []Policy{
		scopedPolicy("scoped-allow-all", VerdictAllow, "sql", `.`, 1, "bot_edge"),
		scopedPolicy("scoped-no-drop", VerdictDeny, "sql", `DROP`, 999, "bot_edge"),
	}
	res := Evaluate(pols, map[string]any{"sql": "DROP TABLE users"})
	if res.Verdict != VerdictDeny || res.MatchedPolicyID != "scoped-no-drop" {
		t.Fatalf("deny beats allow within the scoped tier, got %+v", res)
	}
}

func TestBrokenScopedPolicyFailsClosedInItsTier(t *testing.T) {
	pols := []Policy{
		scopedPolicy("scoped-allow", VerdictAllow, "sql", `^SELECT`, 100, "bot_edge"),
		scopedPolicy("scoped-broken", VerdictDeny, "sql", `(unclosed`, 10, "bot_edge"),
	}
	res := Evaluate(pols, map[string]any{"sql": "SELECT 1"})
	if res.Verdict != VerdictRequireApproval {
		t.Fatalf("a broken scoped gate must fail closed, got %+v", res)
	}
}

func TestSameEffectAttributionByPriority(t *testing.T) {
	pols := []Policy{
		regexPolicy("late", VerdictAllow, "sql", `.`, 200),
		regexPolicy("early", VerdictAllow, "sql", `.`, 5),
	}
	res := Evaluate(pols, map[string]any{"sql": "SELECT 1"})
	if res.MatchedPolicyID != "early" {
		t.Fatalf("lower priority should be credited, got %+v", res)
	}
}

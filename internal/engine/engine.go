// Package engine evaluates an action against the active policy set.
//
// Selection is by candidate keys: wildcards are trailing-only, so an action
// type can only match an enumerable set of patterns (see CandidateKeys).
// Verdicts combine by strict effect precedence — deny > require_approval >
// allow — and anything unmatched or erroring resolves to require_approval
// (fail closed). Priority never changes the verdict; it fixes evaluation
// order, which same-effect policy gets credited, and deny short-circuiting.
package engine

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const (
	VerdictAllow           = "allow"
	VerdictRequireApproval = "require_approval"
	VerdictDeny            = "deny"
)

type Policy struct {
	ID                string
	Name              string
	ActionTypePattern string
	MatcherType       string
	MatcherConfig     map[string]any
	Effect            string
	Priority          int
	Version           int
	BotID             *string // nil = global; set = scoped to one bot (the higher tier)
}

type Result struct {
	Verdict         string
	MatchedPolicyID string   // empty when the fail-closed default applied
	MatcherErrors   []string // non-empty forces at least require_approval in the erroring tier
	OverrodeGlobal  bool     // a bot-scoped rule decided differently than global rules would have
}

// CandidateKeys returns every pattern that could match actionType:
// the exact type, each ancestor namespace with a trailing ".*", and "*".
// "db.admin.drop" → ["db.admin.drop", "db.admin.*", "db.*", "*"].
func CandidateKeys(actionType string) []string {
	keys := []string{actionType}
	segs := strings.Split(actionType, ".")
	for i := len(segs) - 1; i >= 1; i-- {
		keys = append(keys, strings.Join(segs[:i], ".")+".*")
	}
	return append(keys, "*")
}

// Evaluate runs each policy's matcher against the payload and combines fired
// effects in two tiers: the bot's own rules outrank global rules. If any
// bot-scoped policy fires, the scoped tier decides (deny > require_approval >
// allow within it) — so a bot-specific allow pierces a global deny, and a
// bot-specific deny beats a global allow. Global rules decide only when no
// scoped rule fired. A broken matcher counts as require_approval in its own
// tier (fail closed). Policies must already be selected via CandidateKeys.
func Evaluate(policies []Policy, payload map[string]any) Result {
	sorted := make([]Policy, len(policies))
	copy(sorted, policies)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Priority < sorted[j].Priority })

	var res Result
	scoped := map[string]string{} // effect → first policy id that fired with it
	global := map[string]string{}
	for _, p := range sorted {
		fired, err := runMatcher(p, payload)
		effect := p.Effect
		policyID := p.ID
		if err != nil {
			res.MatcherErrors = append(res.MatcherErrors, fmt.Sprintf("%s: %v", p.ID, err))
			effect, policyID = VerdictRequireApproval, "" // broken gate: fail closed, no credit
		} else if !fired {
			continue
		}
		tier := global
		if p.BotID != nil {
			tier = scoped
		}
		if existing, seen := tier[effect]; !seen || (existing == "" && policyID != "") {
			tier[effect] = policyID
		}
	}

	if verdict, id, ok := decideTier(scoped); ok {
		res.Verdict, res.MatchedPolicyID = verdict, id
		if globalVerdict, _, globalFired := decideTier(global); globalFired && globalVerdict != verdict {
			res.OverrodeGlobal = true
		}
		return res
	}
	if verdict, id, ok := decideTier(global); ok {
		res.Verdict, res.MatchedPolicyID = verdict, id
		return res
	}
	res.Verdict = VerdictRequireApproval // silence is a question, not a yes
	return res
}

// decideTier applies effect precedence within one tier.
func decideTier(fired map[string]string) (verdict, policyID string, ok bool) {
	for _, effect := range []string{VerdictDeny, VerdictRequireApproval, VerdictAllow} {
		if id, seen := fired[effect]; seen {
			return effect, id, true
		}
	}
	return "", "", false
}

func runMatcher(p Policy, payload map[string]any) (bool, error) {
	field, _ := p.MatcherConfig["field"].(string)
	value, _ := payload[field].(string)

	switch p.MatcherType {
	case "exact":
		want, ok := p.MatcherConfig["value"].(string)
		if !ok {
			return false, fmt.Errorf("exact matcher missing string %q", "value")
		}
		return value == want, nil
	case "regex":
		pattern, ok := p.MatcherConfig["pattern"].(string)
		if !ok {
			return false, fmt.Errorf("regex matcher missing string %q", "pattern")
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false, fmt.Errorf("compile pattern: %w", err)
		}
		return re.MatchString(value), nil
	default:
		return false, fmt.Errorf("unsupported matcher_type %q", p.MatcherType)
	}
}

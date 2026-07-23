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
}

type Result struct {
	Verdict         string
	MatchedPolicyID string   // empty when the fail-closed default applied
	MatcherErrors   []string // non-empty forces at least require_approval
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

// Evaluate runs each policy's matcher against the payload and combines the
// fired effects by precedence. Policies must already be selected via
// CandidateKeys; type patterns are not re-checked here.
func Evaluate(policies []Policy, payload map[string]any) Result {
	sorted := make([]Policy, len(policies))
	copy(sorted, policies)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Priority < sorted[j].Priority })

	var res Result
	first := map[string]string{} // effect → first policy that fired with it
	for _, p := range sorted {
		fired, err := runMatcher(p, payload)
		if err != nil {
			res.MatcherErrors = append(res.MatcherErrors, fmt.Sprintf("%s: %v", p.ID, err))
			continue
		}
		if !fired {
			continue
		}
		if _, seen := first[p.Effect]; !seen {
			first[p.Effect] = p.ID
		}
		if p.Effect == VerdictDeny {
			break // nothing can override a deny
		}
	}

	switch {
	case first[VerdictDeny] != "":
		res.Verdict, res.MatchedPolicyID = VerdictDeny, first[VerdictDeny]
	case first[VerdictRequireApproval] != "":
		res.Verdict, res.MatchedPolicyID = VerdictRequireApproval, first[VerdictRequireApproval]
	case len(res.MatcherErrors) > 0:
		// A broken policy might have been a gate; never let an allow ride past it.
		res.Verdict = VerdictRequireApproval
	case first[VerdictAllow] != "":
		res.Verdict, res.MatchedPolicyID = VerdictAllow, first[VerdictAllow]
	default:
		res.Verdict = VerdictRequireApproval // silence is a question, not a yes
	}
	return res
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

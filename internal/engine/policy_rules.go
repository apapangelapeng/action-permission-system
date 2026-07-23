package engine

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// PolicyCreateActionType is the action a bot submits to propose a policy.
// It flows through the same queue as everything else, with one hard invariant:
// no policy may auto-allow it, so every proposal passes a human.
const PolicyCreateActionType = "aps.policy.create"

var validEffects = []string{VerdictAllow, VerdictRequireApproval, VerdictDeny}

// ValidatePolicySpec vets a policy at creation time — human-authored or
// bot-proposed. Broken specs are rejected here so the evaluation hot path
// never sees one.
func ValidatePolicySpec(name, pattern, matcherType string, config map[string]any, effect string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("name is required")
	}
	if err := validatePattern(pattern); err != nil {
		return err
	}
	if !slices.Contains(validEffects, effect) {
		return fmt.Errorf("effect must be one of allow, require_approval, deny")
	}

	// The recursion invariant: a policy that would auto-allow policy creation
	// is refused outright — every bot-proposed rule must pass a human.
	if effect == VerdictAllow && slices.Contains(CandidateKeys(PolicyCreateActionType), pattern) {
		return fmt.Errorf("policy creation can never be auto-allowed: a policy matching %q with effect allow would let the bot grant itself rules without human review", pattern)
	}

	switch matcherType {
	case "exact":
		if _, ok := config["value"].(string); !ok {
			return fmt.Errorf("exact matcher needs matcher_config.value (string)")
		}
	case "regex":
		p, ok := config["pattern"].(string)
		if !ok {
			return fmt.Errorf("regex matcher needs matcher_config.pattern (string)")
		}
		if _, err := regexp.Compile(p); err != nil {
			return fmt.Errorf("matcher_config.pattern does not compile: %v", err)
		}
	case "llm":
		return fmt.Errorf("matcher_type llm is reserved but not implemented yet")
	default:
		return fmt.Errorf("matcher_type must be exact or regex")
	}
	if _, ok := config["field"].(string); !ok {
		return fmt.Errorf("matcher_config.field (string) is required")
	}
	return nil
}

// validatePattern enforces trailing-only wildcards — the grammar that keeps
// candidate-key lookup possible: exact ("db.query"), namespace ("db.*"), or "*".
func validatePattern(pattern string) error {
	if pattern == "" {
		return fmt.Errorf("action_type_pattern is required")
	}
	if pattern == "*" {
		return nil
	}
	base, hasWildcard := strings.CutSuffix(pattern, ".*")
	if strings.Contains(base, "*") {
		return fmt.Errorf("wildcards are trailing-only: use %q, %q, or an exact type — not %q", "*", "prefix.*", pattern)
	}
	if hasWildcard && base == "" {
		return fmt.Errorf("invalid pattern %q", pattern)
	}
	return nil
}

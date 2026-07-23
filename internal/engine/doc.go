// Package engine will hold policy evaluation (milestone 1): candidate-key
// type lookup, the Matcher interface (exact, regex; llm later), and effect
// precedence (deny > require_approval > allow; no match => require_approval).
package engine

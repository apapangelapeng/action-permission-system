// demo-bot walks through the whole permission story against a running APS:
// auto-allowed reads, a hard-denied destructive query, a gated write that
// waits for a human, then — tired of asking — it proposes its own policy and,
// if a human approves the rule, enjoys the result.
//
// Run the stack (docker compose up), open the dashboard as alice, then:
//
//	go run ./cmd/demo-bot
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	aps "github.com/apapangelapeng/action-permission-system/sdk/go"
)

func main() {
	base := envOr("APS_URL", "http://localhost:8080")
	key := envOr("APS_BOT_KEY", "aps_demo_bot_key_5f2a9c")
	client := aps.New(base, key)
	ctx := context.Background()

	say("hello — I'm demo-bot, and I ask before I act. (dashboard: %s)", base)

	// 1. Routine reads: covered by the seeded "db-reads-auto" policy.
	say("first, my morning routine — a couple of read queries:")
	for _, sql := range []string{
		"SELECT count(*) FROM shipments WHERE state = 'stuck'",
		"SELECT sku, qty FROM inventory WHERE qty < 10",
	} {
		perform(ctx, client, aps.Action{
			Type:    "db.query",
			Payload: map[string]any{"sql": sql},
			Summary: "routine read",
		})
	}

	// 2. Something destructive: the deny policy stops it cold.
	say("now watch what happens when I get a terrible idea:")
	perform(ctx, client, aps.Action{
		Type:    "db.query",
		Payload: map[string]any{"sql": "DROP TABLE order_archive"},
		Summary: "cleaning up (badly)",
	})

	// 3. A gated write: a human has to say yes, once.
	say("next I need to fix a payment — this touches a sensitive table, so a human decides.")
	say("  ➜ go approve (or deny) me in the Queue tab…")
	perform(ctx, client, aps.Action{
		Type:       "db.query",
		Payload:    map[string]any{"sql": "UPDATE payments SET status = 'resolved' WHERE id = 4242"},
		Summary:    "resolve payment 4242 per ticket #99",
		TTLSeconds: 300,
	})

	// 4. HTTP GETs are not covered by any policy → every one needs a human.
	say("I also check an external API. no policy covers it, so each call bothers a human:")
	say("  ➜ approve me in the Queue tab…")
	perform(ctx, client, aps.Action{
		Type:       "http.request",
		Payload:    map[string]any{"method": "GET", "host": "api.example.com", "path": "/orders/today"},
		Summary:    "fetch today's orders",
		TTLSeconds: 300,
	})

	// 5. Tired of asking, the bot proposes a rule — which itself needs a human.
	say("that gets old fast. so I'm proposing a rule instead of asking every time.")
	say("  ➜ my proposal is now in the Queue — approving it activates the rule.")
	proposal, err := client.ProposePolicy(ctx, aps.PolicySpec{
		Name:              "http-gets-auto",
		Description:       "Proposed by demo-bot: GET requests are read-only and safe to auto-allow.",
		ActionTypePattern: "http.request",
		MatcherType:       "regex",
		MatcherConfig:     map[string]any{"field": "method", "pattern": "^GET$"},
		Effect:            "allow",
	}, "proposal: stop asking me about read-only GETs")
	if err != nil {
		fail(err)
	}
	if !proposal.Allowed() {
		say("my proposal came back %q — fair enough, I'll keep asking. bye!", proposal.Status)
		return
	}
	say("rule approved by %s! let's see it work:", proposal.DecidedBy)

	// 6. The payoff: the same GET now sails through on the bot's own rule.
	perform(ctx, client, aps.Action{
		Type:    "http.request",
		Payload: map[string]any{"method": "GET", "host": "api.example.com", "path": "/orders/today"},
		Summary: "fetch today's orders (again)",
	})

	say("that's the whole loop: ask → human decides → propose a rule → the rule needs a human too.")
	say("every step is in the Audit tab. bye!")
}

// perform runs one action through Check, "executes" it if allowed, and
// narrates the outcome.
func perform(ctx context.Context, client *aps.Client, a aps.Action) {
	start := time.Now()
	req, err := client.Check(ctx, a)
	if err != nil {
		fail(err)
	}
	waited := time.Since(start).Round(time.Second)
	switch {
	case req.Allowed():
		if req.Status == "approved" {
			say("  ✔ approved by %s after %s%s — executing.", req.DecidedBy, waited, note(req))
		} else {
			say("  ✔ auto-allowed by policy %s — executing.", req.MatchedPolicyID)
		}
		// (a real bot would do the actual work right here)
		if _, err := client.ReportExecuted(ctx, req.ID, true, ""); err != nil {
			fail(err)
		}
	case req.Status == "denied":
		say("  ✘ denied%s%s — not doing it.", deniedBy(req), note(req))
	case req.Status == "expired":
		say("  ⏰ nobody answered within the time limit — that counts as a no.")
	default:
		say("  ? unexpected status %q", req.Status)
	}
}

func deniedBy(r *aps.Request) string {
	if r.DecidedBy != "" {
		return " by " + r.DecidedBy
	}
	if r.MatchedPolicyID != "" {
		return " by policy " + r.MatchedPolicyID
	}
	return ""
}

func note(r *aps.Request) string {
	if r.DecisionNote != "" {
		return fmt.Sprintf(" (note: %q)", r.DecisionNote)
	}
	return ""
}

func say(format string, args ...any) {
	fmt.Printf("[demo-bot] "+format+"\n", args...)
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "[demo-bot] error: %v\n", err)
	os.Exit(1)
}

func envOr(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}

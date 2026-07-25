# Action Permission System

A self-hosted permission layer for AI bots: the bot asks before it acts,
policies auto-allow the routine, humans approve the rest, and everything —
every attempt, every decision, every rule change — lands in an audit trail.

Full design: [docs/architecture.html](docs/architecture.html) (open in a browser).

## Quick start

```sh
docker compose up --build
```

Then visit <http://localhost:8080> and sign in (demo accounts below). On
first boot the database is created, migrated, and seeded with demo data
(seeding only happens on an empty database; wipe with `docker compose down -v`
to re-seed).

The dashboard has five screens: **Queue** (pending requests with the raw
payload front and center — the bot's summary is shown as an unverified
caption; policy proposals render as human-readable rules), **Activity**
(every request and its outcome), **Policies** (create and disable rules;
yours take effect immediately), **Audit** (the append-only trail), and
**Controls** (both kill switches: disable a bot, or suspend all auto-allow).
Approvals are first-decision-wins: open two browser windows as alice and bob,
decide the same request in both, and the loser gets told who beat them.

### Demo credentials

| What | Value |
|---|---|
| Dashboard users | `alice` / `password123` · `bob` / `password123` |
| Bot API key | `aps_demo_bot_key_5f2a9c` (bot `demo-bot`) |

The seed includes an active policy set (auto-allowed reads, gated
users/payments queries, a hard deny on destructive SQL), a week of action
history in every lifecycle state, two live pending requests, and a
bot-proposed policy awaiting human review.

Password resets are a SQL one-liner by design (pgcrypto):

```sql
UPDATE users SET password_hash = crypt('new-password', gen_salt('bf')) WHERE username = 'alice';
```

## The five-minute demo

1. `docker compose up --build`, then open <http://localhost:8080> and sign in
   as `alice` / `password123` (keep the Queue tab visible).
2. In another terminal: `go run ./cmd/demo-bot`
3. The bot narrates its way through the whole product: reads sail through on
   a policy, `DROP TABLE` bounces off the deny rule, a payment fix appears in
   your Queue and waits for your click, and when asking gets old the bot
   proposes its own rule — which also lands in your Queue, rendered as a
   human-readable policy. Approve it and watch the bot's next call go through
   without asking. Everything lands in the Audit tab.

For the race demo, open a second window as `bob` and have both users decide
the same request — the second click is told who got there first.

## Using the SDK in your own bot

```go
import aps "github.com/apapangelapeng/action-permission-system/sdk/go"

client := aps.New("http://localhost:8080", os.Getenv("APS_BOT_KEY"))

req, err := client.Check(ctx, aps.Action{
    Type:    "db.query",
    Payload: map[string]any{"sql": query}, // the full truth — this is what humans see
    Summary: "refresh the sales report",   // display-only caption
})
if err != nil { ... }
if req.Allowed() {
    runQuery(query)                                  // do the real work
    client.ReportExecuted(ctx, req.ID, true, "")     // consume the single-use approval
}
```

`Check` blocks while a human decides (a gated action just looks slow to the
bot); `ProposePolicy` submits a rule through the same approval queue. The HTTP
contract is two endpoints, so any language can integrate without the SDK.

## Adding bots

A bot **is** its API key: one key = one bot identity. Every process
presenting the same key acts as the same bot — same policies, same kill
switch, same audit attribution. Mint keys from the dashboard (Controls →
"Create bot + API key") or the API:

```sh
curl -s -X POST localhost:8080/v1/bots -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d '{"name":"deploy-bot"}'
# -> {"id":"bot_…","name":"deploy-bot","api_key":"aps_…"}   # key shown exactly once
```

The new bot works immediately — no enable step — because recognition and
permission are different layers: the key only proves *who* is asking, and
with no policies covering a new bot's actions, everything it tries fails
closed into the human queue. Unknown keys are rejected outright (401);
identities are minted by humans, never self-registered.

### Global vs. bot-scoped policies

A policy applies to **all bots** (global) or to **one bot** (`bot_id` set —
"Applies to" in the policy form). Evaluation sees global rules plus the
acting bot's own; another bot's rules are invisible to it. Bot proposals
are always pinned to the proposing bot — a bot may write rules for itself
(with human approval), never for others; only humans create global rules.
The equivalence check is scope-aware: two bots may hold equivalent rules.
Names are not — a live name means exactly one rule, whatever its scope.

## Poke at it from the terminal

`scripts/test-bot.sh` wraps the API so you can play both roles by hand —
submit actions as the bot, decide them as alice or bob:

```sh
./scripts/test-bot.sh read       # SELECT → auto-allowed instantly
./scripts/test-bot.sh drop       # DROP TABLE → denied instantly
./scripts/test-bot.sh write      # UPDATE payments → pending, waits for a human
./scripts/test-bot.sh email      # email.send → pending (fail-closed default)
./scripts/test-bot.sh propose    # bot proposes a policy → lands in the queue

./scripts/test-bot.sh queue                      # what's waiting for a human
./scripts/test-bot.sh poll <id>                  # state of one request
./scripts/test-bot.sh approve <id> alice looks fine
./scripts/test-bot.sh deny <id> bob too risky
./scripts/test-bot.sh done <id>                  # bot reports executed (consumes approval)
```

Things worth trying: decide the same id twice (409 — first decision wins),
`done` twice (409 — approvals are single-use), `propose` then approve the
proposal and watch `email` start auto-allowing, and let a pending request sit
15 minutes to see TTL expiry count as a no. `APS_URL` and `APS_BOT_KEY`
override the defaults.

## Try the decision loop (curl)

```sh
KEY="aps_demo_bot_key_5f2a9c"

# Auto-allowed by policy: instant verdict
curl -s -X POST localhost:8080/v1/actions -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"type":"db.query","payload":{"sql":"SELECT id FROM orders LIMIT 5"}}'

# Gated: goes pending, returns an id to poll
curl -s -X POST localhost:8080/v1/actions -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d '{"type":"db.query","payload":{"sql":"UPDATE payments SET status=1 WHERE id=7"}}'

# A human logs in and decides
TOKEN=$(curl -s -X POST localhost:8080/v1/login -H 'Content-Type: application/json' \
  -d '{"username":"alice","password":"password123"}' | python3 -c 'import json,sys;print(json.load(sys.stdin)["token"])')
curl -s -X POST localhost:8080/v1/actions/<id>/decision -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d '{"decision":"approve","note":"looks right"}'

# Bot polls, then consumes the approval (single-use)
curl -s localhost:8080/v1/actions/<id> -H "X-API-Key: $KEY"
curl -s -X POST localhost:8080/v1/actions/<id>/executed -H "X-API-Key: $KEY" \
  -H 'Content-Type: application/json' -d '{"success":true}'
```

Semantics: `deny` > `require_approval` > `allow`; no matching policy ⇒ a human
decides (fail closed). First decision wins (a second decider gets 409).
Requests expire after their TTL (default 15 min; the bot may request longer up
front, capped by `max_ttl_seconds`). Approvals are single-use. Disabled bots
get deny-all; setting `auto_allow_suspended` to `true` in `system_settings`
sends every action back to a human.

### The bot can propose its own rules

Policy creation is just another action. The bot submits `aps.policy.create`
with the proposed rule as its payload; it lands in the human queue like
everything else (a hard invariant refuses any policy that would auto-allow
`aps.policy.create` — every proposal passes a human). Approving the request
activates the rule; denying rejects it; letting it expire rejects it too.
Disabling a policy cascades to any bot policies it authorized.

Duplicates are refused two ways. If a policy with the same pattern, matcher,
and effect is already active or awaiting review, creating another one — bot
proposal or human — gets a 409 pointing at the existing rule; the name is
ignored on purpose, since renaming a rule doesn't make it a new rule. A
**reused name** is refused the same way (case- and whitespace-insensitive),
because the name is how a human says which rule to turn off. (Disabled and
rejected policies block neither check, so a retired rule can be re-proposed
and its name reused.)

A refused policy is never stored, so it never shows up in the Policies tab
or the queue — the only record is a `policy.duplicate_refused` audit event
naming the rule it collided with, visible in the Audit tab and the server
log.

```sh
curl -s -X POST localhost:8080/v1/actions -H "X-API-Key: $KEY" -H 'Content-Type: application/json' -d '{
  "type": "aps.policy.create",
  "payload": {
    "name": "db-reads-auto", "description": "SELECTs are read-only",
    "action_type_pattern": "db.query",
    "matcher_type": "regex", "matcher_config": {"field": "sql", "pattern": "(?i)^\\s*SELECT"},
    "effect": "allow"
  },
  "summary": "proposal: stop asking me about reads"
}'
```

## Development

```sh
docker compose up db -d           # just postgres
go run ./cmd/aps                  # api + embedded dashboard on :8080

cd web && npm install
npm run dev                       # svelte dev server on :5173, proxies /v1 + /healthz to :8080
npm run build                     # regenerates web/dist (committed; embedded into the Go binary)
```

## Layout

```
cmd/aps/            main: serves API + embedded dashboard
cmd/demo-bot/       narrated example bot built on the SDK
internal/api/       HTTP handlers
internal/engine/    policy evaluation: candidate-key lookup, matchers, precedence
internal/store/     postgres access, migrations, seed — the only SQL in the codebase
internal/auth/      credential hashing, session tokens
web/                Svelte dashboard; built output in web/dist is embedded
sdk/go/             bot-side SDK: Check, ReportExecuted, ProposePolicy
docs/               architecture doc
```

## Status

MVP complete — all five milestones of the [plan](docs/architecture.html#mvp):
decision loop, dashboard, policy creation in both directions, SDK, and the
demo bot. Designed-but-deferred: LLM judge matcher, webhooks/notifications,
idempotency keys, rate limiting, role tiers, policy versioning UI (see the
architecture doc's open-questions section).

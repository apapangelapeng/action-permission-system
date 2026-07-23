# Action Permission System

A self-hosted permission layer for AI bots: the bot asks before it acts,
policies auto-allow the routine, humans approve the rest, and everything —
every attempt, every decision, every rule change — lands in an audit trail.

Full design: [docs/architecture.html](docs/architecture.html) (open in a browser).

## Quick start

```sh
docker compose up --build
```

Then visit <http://localhost:8080>. On first boot the database is created,
migrated, and seeded with demo data (seeding only happens on an empty
database; wipe with `docker compose down -v` to re-seed).

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
internal/api/       HTTP handlers
internal/engine/    policy evaluation (milestone 1)
internal/store/     postgres access, migrations, seed — the only SQL in the codebase
internal/auth/      user sessions + bot API keys (milestones 1–2)
web/                Svelte dashboard; built output in web/dist is embedded
sdk/go/             bot-side SDK (milestone 4)
docs/               architecture doc
```

## Status

Milestone 1 of the [MVP plan](docs/architecture.html#mvp): the decision loop
works end-to-end over curl — submit, evaluate, approve/deny, execute, expire,
audit. Milestone 2 (the dashboard) is next.

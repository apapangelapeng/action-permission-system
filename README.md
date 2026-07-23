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

Milestone 0 of the [MVP plan](docs/architecture.html#mvp): scaffold, schema
(all seven tables), seed data, health check. The decision API is milestone 1.

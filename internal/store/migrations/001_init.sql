-- Full schema per docs/architecture.html. All seven tables land in milestone 0
-- so later milestones add features without further migrations.
-- pgcrypto provides crypt()/gen_salt() so a password can be reset with plain
-- SQL: UPDATE users SET password_hash = crypt('new', gen_salt('bf')) WHERE username = '...';

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'approver',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    disabled_at   TIMESTAMPTZ
);

CREATE TABLE bots (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL UNIQUE,
    api_key_hash TEXT NOT NULL UNIQUE,
    created_by   TEXT NOT NULL REFERENCES users(id),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    disabled_at  TIMESTAMPTZ
);

CREATE TABLE policies (
    id                      TEXT PRIMARY KEY,
    name                    TEXT NOT NULL,
    description             TEXT NOT NULL,
    action_type_pattern     TEXT NOT NULL,
    matcher_type            TEXT NOT NULL CHECK (matcher_type IN ('exact', 'regex', 'llm')),
    matcher_config          JSONB NOT NULL,
    effect                  TEXT NOT NULL CHECK (effect IN ('allow', 'require_approval', 'deny')),
    priority                INTEGER NOT NULL DEFAULT 100,
    status                  TEXT NOT NULL CHECK (status IN ('draft', 'pending_approval', 'active', 'disabled', 'rejected')),
    version                 INTEGER NOT NULL DEFAULT 1,
    supersedes_policy_id    TEXT REFERENCES policies(id),
    created_by_kind         TEXT NOT NULL CHECK (created_by_kind IN ('human', 'bot')),
    created_by_id           TEXT NOT NULL,
    authorized_by_policy_id TEXT REFERENCES policies(id),
    depth                   INTEGER NOT NULL DEFAULT 0,
    approved_by             TEXT REFERENCES users(id),
    approved_at             TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- every active policy names its human
    CONSTRAINT active_policies_are_approved
        CHECK (status <> 'active' OR approved_by IS NOT NULL)
);

CREATE INDEX idx_policies_lookup ON policies (status, action_type_pattern);

CREATE TABLE action_requests (
    id                TEXT PRIMARY KEY,
    bot_id            TEXT NOT NULL REFERENCES bots(id),
    action_type       TEXT NOT NULL,
    payload           JSONB NOT NULL,
    summary           TEXT,
    idempotency_key   TEXT,
    matched_policy_id TEXT REFERENCES policies(id),
    verdict           TEXT NOT NULL CHECK (verdict IN ('allow', 'require_approval', 'deny')),
    status            TEXT NOT NULL CHECK (status IN ('auto_allowed', 'pending', 'approved', 'denied', 'expired', 'executed', 'failed')),
    decided_by        TEXT REFERENCES users(id),
    decided_at        TIMESTAMPTZ,
    decision_note     TEXT,
    ttl_seconds       INTEGER NOT NULL DEFAULT 900,
    expires_at        TIMESTAMPTZ NOT NULL,
    executed_at       TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_action_requests_idempotency
    ON action_requests (bot_id, idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX idx_action_requests_status ON action_requests (status);
CREATE INDEX idx_action_requests_created ON action_requests (created_at DESC);

-- Append-only: the application layer never issues UPDATE or DELETE here.
CREATE TABLE audit_events (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    ts           TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor_kind   TEXT NOT NULL CHECK (actor_kind IN ('human', 'bot', 'system')),
    actor_id     TEXT,
    event_type   TEXT NOT NULL,
    subject_type TEXT NOT NULL,
    subject_id   TEXT NOT NULL,
    details      JSONB NOT NULL DEFAULT '{}'
);

CREATE INDEX idx_audit_events_subject ON audit_events (subject_type, subject_id);
CREATE INDEX idx_audit_events_ts ON audit_events (ts DESC);

CREATE TABLE sessions (
    token_hash TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_sessions_expires ON sessions (expires_at);

CREATE TABLE system_settings (
    key        TEXT PRIMARY KEY,
    value      JSONB NOT NULL,
    updated_by TEXT REFERENCES users(id),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

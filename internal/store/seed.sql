-- Demo data: two humans, one bot, a realistic policy set (active, pending,
-- rejected, disabled), a week of action history in every lifecycle state, and
-- the audit events tying it together. Timestamps are relative to now() so the
-- dashboard always looks alive. Applied only when the users table is empty.
--
-- Logins:   alice / password123   ·   bob / password123
-- Bot key:  aps_demo_bot_key_5f2a9c   (SHA-256 stored below)

INSERT INTO users (id, name, username, password_hash, created_at) VALUES
    ('usr_alice', 'Alice Zhang', 'alice', crypt('password123', gen_salt('bf')), now() - interval '30 days'),
    ('usr_bob',   'Bob Ramirez', 'bob',   crypt('password123', gen_salt('bf')), now() - interval '28 days');

INSERT INTO bots (id, name, api_key_hash, created_by, created_at) VALUES
    ('bot_demo', 'demo-bot', encode(digest('aps_demo_bot_key_5f2a9c', 'sha256'), 'hex'), 'usr_alice', now() - interval '21 days');

INSERT INTO system_settings (key, value, updated_by, updated_at) VALUES
    ('auto_allow_suspended', 'false', 'usr_alice', now() - interval '21 days'),
    ('max_ttl_seconds',      '21600', 'usr_alice', now() - interval '21 days'),
    ('policy_depth_limit',   '3',     'usr_alice', now() - interval '21 days');

-- ── Policies ────────────────────────────────────────────────────────────────

INSERT INTO policies (id, name, description, action_type_pattern, matcher_type, matcher_config,
                      effect, priority, status, created_by_kind, created_by_id, depth,
                      approved_by, approved_at, created_at) VALUES
    ('pol_no_destruction', 'no-destruction',
     'Destructive SQL is never allowed, no matter what other rules say.',
     'db.*', 'regex', '{"field": "sql", "pattern": "(?i)\\b(DROP|TRUNCATE)\\b"}',
     'deny', 10, 'active', 'human', 'usr_bob', 0,
     'usr_bob', now() - interval '20 days', now() - interval '20 days'),

    ('pol_sensitive_tables', 'sensitive-tables-gated',
     'Any query touching users or payments needs a human, even reads.',
     'db.query', 'regex', '{"field": "sql", "pattern": "(?i)\\b(users|payments)\\b"}',
     'require_approval', 50, 'active', 'human', 'usr_alice', 0,
     'usr_alice', now() - interval '20 days', now() - interval '20 days'),

    ('pol_db_reads_auto', 'db-reads-auto',
     'Plain SELECT queries are safe to run without asking.',
     'db.query', 'regex', '{"field": "sql", "pattern": "(?i)^\\s*SELECT\\b"}',
     'allow', 100, 'active', 'human', 'usr_alice', 0,
     'usr_alice', now() - interval '14 days', now() - interval '14 days'),

    ('pol_status_page_ok', 'status-page-writes',
     'Posting to our own status page is routine.',
     'http.request', 'exact', '{"field": "host", "value": "status.internal.example.com"}',
     'allow', 100, 'active', 'human', 'usr_bob', 0,
     'usr_bob', now() - interval '10 days', now() - interval '10 days');

-- A policy the bot proposed that a human has NOT yet reviewed (pairs with the
-- pending aps.policy.create request below — milestone 3's review queue).
INSERT INTO policies (id, name, description, action_type_pattern, matcher_type, matcher_config,
                      effect, priority, status, created_by_kind, created_by_id, depth, created_at) VALUES
    ('pol_bot_http_get', 'http-gets-auto',
     'Proposed by demo-bot: GET requests are read-only and safe to auto-allow.',
     'http.request', 'regex', '{"field": "method", "pattern": "^GET$"}',
     'allow', 100, 'pending_approval', 'bot', 'bot_demo', 1,
     now() - interval '25 minutes');

-- A bot proposal a human rejected — the review story's other ending.
INSERT INTO policies (id, name, description, action_type_pattern, matcher_type, matcher_config,
                      effect, priority, status, created_by_kind, created_by_id, depth, created_at) VALUES
    ('pol_bot_allow_all', 'allow-everything',
     'Proposed by demo-bot: allow all actions to reduce approval fatigue.',
     '*', 'regex', '{"field": "", "pattern": ".*"}',
     'allow', 100, 'rejected', 'bot', 'bot_demo', 1,
     now() - interval '6 days');

-- A retired human policy, so the lifecycle view has a 'disabled' example.
INSERT INTO policies (id, name, description, action_type_pattern, matcher_type, matcher_config,
                      effect, priority, status, created_by_kind, created_by_id, depth,
                      approved_by, approved_at, created_at) VALUES
    ('pol_old_email_gate', 'email-gated (retired)',
     'All outbound email needed approval; retired after the pilot.',
     'email.send', 'regex', '{"field": "to", "pattern": ".*"}',
     'require_approval', 100, 'disabled', 'human', 'usr_alice', 0,
     'usr_alice', now() - interval '18 days', now() - interval '18 days');

-- ── Action history ──────────────────────────────────────────────────────────

INSERT INTO action_requests (id, bot_id, action_type, payload, summary, matched_policy_id,
                             verdict, status, decided_by, decided_at, decision_note,
                             ttl_seconds, expires_at, executed_at, created_at) VALUES
    -- Routine auto-allowed reads over the past days
    ('act_sel_1', 'bot_demo', 'db.query',
     '{"sql": "SELECT id, status FROM orders WHERE created_at > now() - interval ''1 day''"}',
     'Fetch yesterday''s orders', 'pol_db_reads_auto',
     'allow', 'executed', NULL, NULL, NULL,
     900, now() - interval '2 days' + interval '15 minutes', now() - interval '2 days' + interval '3 seconds', now() - interval '2 days'),

    ('act_sel_2', 'bot_demo', 'db.query',
     '{"sql": "SELECT count(*) FROM shipments WHERE state = ''stuck''"}',
     'Count stuck shipments', 'pol_db_reads_auto',
     'allow', 'executed', NULL, NULL, NULL,
     900, now() - interval '1 day' + interval '15 minutes', now() - interval '1 day' + interval '2 seconds', now() - interval '1 day'),

    ('act_sel_3', 'bot_demo', 'db.query',
     '{"sql": "SELECT sku, qty FROM inventory WHERE qty < 10"}',
     'Low-inventory check', 'pol_db_reads_auto',
     'allow', 'executed', NULL, NULL, NULL,
     900, now() - interval '3 hours' + interval '15 minutes', now() - interval '3 hours' + interval '2 seconds', now() - interval '3 hours'),

    -- A gated write Alice approved yesterday (single-use, consumed)
    ('act_upd_1', 'bot_demo', 'db.query',
     '{"sql": "UPDATE payments SET status = ''refunded'' WHERE id = 8841"}',
     'Refund payment 8841 per support ticket #512', 'pol_sensitive_tables',
     'require_approval', 'executed', 'usr_alice', now() - interval '1 day' + interval '4 minutes',
     'One-off refund, verified against the ticket. Propose a policy if this recurs.',
     900, now() - interval '1 day' + interval '15 minutes', now() - interval '1 day' + interval '5 minutes', now() - interval '1 day'),

    -- Auto-denied destructive query (deny needs no human)
    ('act_drop_1', 'bot_demo', 'db.query',
     '{"sql": "DROP TABLE order_archive"}',
     'Clean up old archive table', 'pol_no_destruction',
     'deny', 'denied', NULL, NULL, NULL,
     900, now() - interval '6 hours' + interval '15 minutes', NULL, now() - interval '6 hours'),

    -- Nobody answered in time → expired (fail closed)
    ('act_email_1', 'bot_demo', 'email.send',
     '{"to": "vip@example.com", "subject": "Your order shipped", "body": "Tracking: ZX8842..."}',
     'Shipping notification to customer', NULL,
     'require_approval', 'expired', NULL, NULL, NULL,
     900, now() - interval '1 day 2 hours' + interval '15 minutes', NULL, now() - interval '1 day 2 hours'),

    -- Bob denied an outbound API call with a note
    ('act_http_1', 'bot_demo', 'http.request',
     '{"method": "POST", "host": "api.stripe.com", "path": "/v1/refunds", "body": {"charge": "ch_3P8x", "amount": 1999}}',
     'Refund charge ch_3P8x via Stripe', NULL,
     'require_approval', 'denied', 'usr_bob', now() - interval '2 hours' + interval '6 minutes',
     'Refunds go through the payments team, not direct Stripe calls.',
     900, now() - interval '2 hours' + interval '15 minutes', NULL, now() - interval '2 hours'),

    -- Approved but the bot reported failure executing it
    ('act_upd_2', 'bot_demo', 'db.query',
     '{"sql": "UPDATE users SET email_verified = true WHERE id = 3301"}',
     'Mark user 3301 verified after manual check', 'pol_sensitive_tables',
     'require_approval', 'failed', 'usr_alice', now() - interval '3 days' + interval '2 minutes',
     'OK per identity check.',
     900, now() - interval '3 days' + interval '15 minutes', now() - interval '3 days' + interval '3 minutes', now() - interval '3 days'),

    -- LIVE: two requests sitting in the queue right now
    ('act_upd_3', 'bot_demo', 'db.query',
     '{"sql": "UPDATE payments SET status = ''disputed'' WHERE id = 9107"}',
     'Flag payment 9107 as disputed', 'pol_sensitive_tables',
     'require_approval', 'pending', NULL, NULL, NULL,
     900, now() + interval '10 minutes', NULL, now() - interval '5 minutes'),

    ('act_http_2', 'bot_demo', 'http.request',
     '{"method": "POST", "host": "hooks.slack.com", "path": "/services/T00/B00/xyz", "body": {"text": "Daily ops summary: 3 stuck shipments"}}',
     'Post ops summary to Slack', NULL,
     'require_approval', 'pending', NULL, NULL, NULL,
     900, now() + interval '12 minutes', NULL, now() - interval '3 minutes'),

    -- LIVE: the bot's policy proposal, waiting for review (longer TTL requested)
    ('act_polcreate_1', 'bot_demo', 'aps.policy.create',
     '{"policy_id": "pol_bot_http_get", "name": "http-gets-auto", "action_type_pattern": "http.request", "matcher_type": "regex", "matcher_config": {"field": "method", "pattern": "^GET$"}, "effect": "allow", "reason": "GETs are read-only; I made 14 of them this week and each needed an approval."}',
     'Proposal: auto-allow read-only GET requests', NULL,
     'require_approval', 'pending', NULL, NULL, NULL,
     3600, now() + interval '35 minutes', NULL, now() - interval '25 minutes');

-- ── Audit trail (append-only mirror of everything above) ────────────────────

INSERT INTO audit_events (ts, actor_kind, actor_id, event_type, subject_type, subject_id, details) VALUES
    (now() - interval '30 days', 'system', NULL,        'user.created',      'user',   'usr_alice', '{"username": "alice"}'),
    (now() - interval '28 days', 'system', NULL,        'user.created',      'user',   'usr_bob',   '{"username": "bob"}'),
    (now() - interval '21 days', 'human', 'usr_alice',  'bot.created',       'bot',    'bot_demo',  '{"name": "demo-bot"}'),

    (now() - interval '20 days', 'human', 'usr_bob',    'policy.created',    'policy', 'pol_no_destruction',   '{"effect": "deny"}'),
    (now() - interval '20 days', 'human', 'usr_bob',    'policy.activated',  'policy', 'pol_no_destruction',   '{}'),
    (now() - interval '20 days', 'human', 'usr_alice',  'policy.created',    'policy', 'pol_sensitive_tables', '{"effect": "require_approval"}'),
    (now() - interval '20 days', 'human', 'usr_alice',  'policy.activated',  'policy', 'pol_sensitive_tables', '{}'),
    (now() - interval '18 days', 'human', 'usr_alice',  'policy.created',    'policy', 'pol_old_email_gate',   '{"effect": "require_approval"}'),
    (now() - interval '18 days', 'human', 'usr_alice',  'policy.activated',  'policy', 'pol_old_email_gate',   '{}'),
    (now() - interval '14 days', 'human', 'usr_alice',  'policy.created',    'policy', 'pol_db_reads_auto',    '{"effect": "allow"}'),
    (now() - interval '14 days', 'human', 'usr_alice',  'policy.activated',  'policy', 'pol_db_reads_auto',    '{}'),
    (now() - interval '10 days', 'human', 'usr_bob',    'policy.created',    'policy', 'pol_status_page_ok',   '{"effect": "allow"}'),
    (now() - interval '10 days', 'human', 'usr_bob',    'policy.activated',  'policy', 'pol_status_page_ok',   '{}'),

    (now() - interval '6 days',  'bot', 'bot_demo',     'policy.proposed',   'policy', 'pol_bot_allow_all', '{"effect": "allow", "pattern": "*"}'),
    (now() - interval '6 days' + interval '40 minutes', 'human', 'usr_bob', 'policy.rejected', 'policy', 'pol_bot_allow_all', '{"note": "Absolutely not — this would disable the entire system."}'),
    (now() - interval '8 days',  'human', 'usr_alice',  'policy.disabled',   'policy', 'pol_old_email_gate', '{"note": "Pilot over; email volume too high for per-send review."}'),

    (now() - interval '2 days',  'bot', 'bot_demo',     'action.submitted',  'action_request', 'act_sel_1', '{"type": "db.query"}'),
    (now() - interval '2 days',  'system', NULL,        'action.allowed',    'action_request', 'act_sel_1', '{"policy": "pol_db_reads_auto", "version": 1}'),
    (now() - interval '2 days' + interval '3 seconds', 'bot', 'bot_demo', 'action.executed', 'action_request', 'act_sel_1', '{}'),
    (now() - interval '1 day',   'bot', 'bot_demo',     'action.submitted',  'action_request', 'act_sel_2', '{"type": "db.query"}'),
    (now() - interval '1 day',   'system', NULL,        'action.allowed',    'action_request', 'act_sel_2', '{"policy": "pol_db_reads_auto", "version": 1}'),
    (now() - interval '1 day' + interval '2 seconds', 'bot', 'bot_demo', 'action.executed', 'action_request', 'act_sel_2', '{}'),
    (now() - interval '3 hours', 'bot', 'bot_demo',     'action.submitted',  'action_request', 'act_sel_3', '{"type": "db.query"}'),
    (now() - interval '3 hours', 'system', NULL,        'action.allowed',    'action_request', 'act_sel_3', '{"policy": "pol_db_reads_auto", "version": 1}'),
    (now() - interval '3 hours' + interval '2 seconds', 'bot', 'bot_demo', 'action.executed', 'action_request', 'act_sel_3', '{}'),

    (now() - interval '1 day',   'bot', 'bot_demo',     'action.submitted',  'action_request', 'act_upd_1', '{"type": "db.query"}'),
    (now() - interval '1 day',   'system', NULL,        'action.pending',    'action_request', 'act_upd_1', '{"policy": "pol_sensitive_tables", "version": 1}'),
    (now() - interval '1 day' + interval '4 minutes', 'human', 'usr_alice', 'action.approved', 'action_request', 'act_upd_1', '{"note": "One-off refund, verified against the ticket. Propose a policy if this recurs."}'),
    (now() - interval '1 day' + interval '5 minutes', 'bot', 'bot_demo',  'action.executed', 'action_request', 'act_upd_1', '{}'),

    (now() - interval '6 hours', 'bot', 'bot_demo',     'action.submitted',  'action_request', 'act_drop_1', '{"type": "db.query"}'),
    (now() - interval '6 hours', 'system', NULL,        'action.denied',     'action_request', 'act_drop_1', '{"policy": "pol_no_destruction", "version": 1, "auto": true}'),

    (now() - interval '1 day 2 hours', 'bot', 'bot_demo', 'action.submitted', 'action_request', 'act_email_1', '{"type": "email.send"}'),
    (now() - interval '1 day 2 hours', 'system', NULL,  'action.pending',    'action_request', 'act_email_1', '{"default": "fail_closed"}'),
    (now() - interval '1 day 1 hour 45 minutes', 'system', NULL, 'action.expired', 'action_request', 'act_email_1', '{"ttl_seconds": 900}'),

    (now() - interval '2 hours', 'bot', 'bot_demo',     'action.submitted',  'action_request', 'act_http_1', '{"type": "http.request"}'),
    (now() - interval '2 hours', 'system', NULL,        'action.pending',    'action_request', 'act_http_1', '{"default": "fail_closed"}'),
    (now() - interval '2 hours' + interval '6 minutes', 'human', 'usr_bob', 'action.denied', 'action_request', 'act_http_1', '{"note": "Refunds go through the payments team, not direct Stripe calls."}'),

    (now() - interval '3 days', 'bot', 'bot_demo',      'action.submitted',  'action_request', 'act_upd_2', '{"type": "db.query"}'),
    (now() - interval '3 days', 'system', NULL,         'action.pending',    'action_request', 'act_upd_2', '{"policy": "pol_sensitive_tables", "version": 1}'),
    (now() - interval '3 days' + interval '2 minutes', 'human', 'usr_alice', 'action.approved', 'action_request', 'act_upd_2', '{"note": "OK per identity check."}'),
    (now() - interval '3 days' + interval '3 minutes', 'bot', 'bot_demo',  'action.failed',   'action_request', 'act_upd_2', '{"error": "db timeout after 30s"}'),

    (now() - interval '5 minutes', 'bot', 'bot_demo',   'action.submitted',  'action_request', 'act_upd_3', '{"type": "db.query"}'),
    (now() - interval '5 minutes', 'system', NULL,      'action.pending',    'action_request', 'act_upd_3', '{"policy": "pol_sensitive_tables", "version": 1}'),
    (now() - interval '3 minutes', 'bot', 'bot_demo',   'action.submitted',  'action_request', 'act_http_2', '{"type": "http.request"}'),
    (now() - interval '3 minutes', 'system', NULL,      'action.pending',    'action_request', 'act_http_2', '{"default": "fail_closed"}'),

    (now() - interval '25 minutes', 'bot', 'bot_demo',  'action.submitted',  'action_request', 'act_polcreate_1', '{"type": "aps.policy.create", "ttl_requested": 3600}'),
    (now() - interval '25 minutes', 'system', NULL,     'action.pending',    'action_request', 'act_polcreate_1', '{"default": "fail_closed"}'),
    (now() - interval '25 minutes', 'bot', 'bot_demo',  'policy.proposed',   'policy', 'pol_bot_http_get', '{"via_action": "act_polcreate_1"}');

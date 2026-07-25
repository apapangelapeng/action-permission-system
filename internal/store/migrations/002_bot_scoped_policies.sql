-- Policies can be global (bot_id NULL — applies to every bot) or scoped to a
-- single bot. Bot-proposed policies are always scoped to the proposing bot:
-- a bot may write rules for itself (with human approval), never for others.
ALTER TABLE policies ADD COLUMN bot_id TEXT REFERENCES bots(id);
CREATE INDEX idx_policies_bot ON policies (bot_id);

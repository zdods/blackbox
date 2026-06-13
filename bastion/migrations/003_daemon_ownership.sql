-- Associate each daemon with the user that registered it, so the daemon and
-- file-proxy endpoints can scope access to the authenticated user. Single-user
-- today (every daemon belongs to the one account), but this prevents a
-- cross-tenant IDOR if multi-user is ever added — cheaper to enforce now than
-- after the single-tenant assumption hardens.
ALTER TABLE daemons ADD COLUMN IF NOT EXISTS owner_id UUID REFERENCES users(id) ON DELETE CASCADE;

-- Backfill existing rows to the single account (there is exactly one).
UPDATE daemons SET owner_id = (SELECT id FROM users ORDER BY created_at LIMIT 1)
WHERE owner_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_daemons_owner ON daemons(owner_id);

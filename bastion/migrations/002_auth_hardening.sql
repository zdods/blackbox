-- Session revocation: bumping token_version invalidates all outstanding JWTs for the user.
ALTER TABLE users ADD COLUMN IF NOT EXISTS token_version INT NOT NULL DEFAULT 1;

-- Daemon tokens at rest: store a SHA-256 hash instead of the plaintext token.
-- Existing plaintext tokens are backfilled into token_hash by the server at
-- startup (see RunMigrations), then the plaintext column is cleared.
ALTER TABLE daemons ADD COLUMN IF NOT EXISTS token_hash TEXT;
ALTER TABLE daemons ALTER COLUMN token DROP NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_daemons_token_hash ON daemons(token_hash);
DROP INDEX IF EXISTS idx_daemons_token;

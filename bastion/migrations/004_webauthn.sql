-- Passkey (WebAuthn) authentication. Passkeys replace only the identity-proving
-- step: after a successful ceremony the server issues the same JWT session as
-- the password+TOTP path. A passkey-only account has no password, so make the
-- password nullable. (GetUserByID/GetUserByUsername COALESCE it to '', and an
-- empty hash makes CheckPassword fail closed, so the password login naturally
-- rejects passkey-only accounts.)
ALTER TABLE users ALTER COLUMN password_hash DROP NOT NULL;

CREATE TABLE IF NOT EXISTS webauthn_credentials (
    -- Surrogate PK used in management URLs (/api/passkeys/{id}); the binary
    -- credential_id never appears in a URL.
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    credential_id    BYTEA NOT NULL UNIQUE,        -- webauthn.Credential.ID (raw bytes)
    public_key       BYTEA NOT NULL,               -- COSE public key bytes
    attestation_type TEXT   NOT NULL DEFAULT '',
    transports       TEXT[] NOT NULL DEFAULT '{}', -- []protocol.AuthenticatorTransport as strings
    aaguid           BYTEA,                         -- authenticator model id; may be all-zero
    sign_count       BIGINT  NOT NULL DEFAULT 0,    -- uint32 -> BIGINT (int4 would overflow)
    backup_eligible  BOOLEAN NOT NULL DEFAULT false,
    backup_state     BOOLEAN NOT NULL DEFAULT false,
    clone_warning    BOOLEAN NOT NULL DEFAULT false,
    name             TEXT NOT NULL DEFAULT '',      -- friendly label shown in the UI
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_webauthn_credentials_user ON webauthn_credentials(user_id);

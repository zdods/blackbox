package main

import (
	"context"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PasskeyMeta is the non-secret view of a stored credential for the management UI.
type PasskeyMeta struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Transports  []string   `json:"transports"`
	CreatedAt   time.Time  `json:"created_at"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	BackupState bool       `json:"backup_state"`
}

// credColumns is the column list used when scanning a webauthn.Credential.
const credColumns = `credential_id, public_key, attestation_type, transports, aaguid, sign_count, backup_eligible, backup_state, clone_warning`

// scanCredential reads one credential row (in credColumns order) into a
// webauthn.Credential.
func scanCredential(row pgx.Row) (webauthn.Credential, error) {
	var (
		c          webauthn.Credential
		transports []string
		signCount  int64
	)
	err := row.Scan(&c.ID, &c.PublicKey, &c.AttestationType, &transports, &c.Authenticator.AAGUID,
		&signCount, &c.Flags.BackupEligible, &c.Flags.BackupState, &c.Authenticator.CloneWarning)
	if err != nil {
		return webauthn.Credential{}, err
	}
	c.Transport = stringsToTransports(transports)
	c.Authenticator.SignCount = uint32(signCount)
	return c, nil
}

// ListCredentialsByUser loads all webauthn credentials for a user.
func ListCredentialsByUser(ctx context.Context, pool *pgxpool.Pool, userID string) ([]webauthn.Credential, error) {
	rows, err := pool.Query(ctx, `SELECT `+credColumns+` FROM webauthn_credentials WHERE user_id = $1 ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var creds []webauthn.Credential
	for rows.Next() {
		c, err := scanCredential(rows)
		if err != nil {
			return nil, err
		}
		creds = append(creds, c)
	}
	return creds, rows.Err()
}

// InsertCredential stores a newly registered credential for an existing user.
func InsertCredential(ctx context.Context, pool *pgxpool.Pool, userID string, c *webauthn.Credential, name string) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO webauthn_credentials
		 (user_id, credential_id, public_key, attestation_type, transports, aaguid, sign_count, backup_eligible, backup_state, clone_warning, name)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		userID, c.ID, c.PublicKey, c.AttestationType, transportsToStrings(c.Transport), c.Authenticator.AAGUID,
		int64(c.Authenticator.SignCount), c.Flags.BackupEligible, c.Flags.BackupState, c.Authenticator.CloneWarning, name,
	)
	return err
}

// CreateUserWithCredential creates the first (passkey-only) user and its initial
// credential atomically. userID is the pre-generated handle baked into the
// registration ceremony.
func CreateUserWithCredential(ctx context.Context, pool *pgxpool.Pool, userID, username string, c *webauthn.Credential, name string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx,
		`INSERT INTO users (id, username) VALUES ($1, $2)`, userID, username); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO webauthn_credentials
		 (user_id, credential_id, public_key, attestation_type, transports, aaguid, sign_count, backup_eligible, backup_state, clone_warning, name)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		userID, c.ID, c.PublicKey, c.AttestationType, transportsToStrings(c.Transport), c.Authenticator.AAGUID,
		int64(c.Authenticator.SignCount), c.Flags.BackupEligible, c.Flags.BackupState, c.Authenticator.CloneWarning, name,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ListPasskeyMeta returns the management-facing metadata for a user's passkeys.
func ListPasskeyMeta(ctx context.Context, pool *pgxpool.Pool, userID string) ([]PasskeyMeta, error) {
	rows, err := pool.Query(ctx,
		`SELECT id::text, name, transports, created_at, last_used_at, backup_state
		 FROM webauthn_credentials WHERE user_id = $1 ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	metas := []PasskeyMeta{}
	for rows.Next() {
		var m PasskeyMeta
		if err := rows.Scan(&m.ID, &m.Name, &m.Transports, &m.CreatedAt, &m.LastUsedAt, &m.BackupState); err != nil {
			return nil, err
		}
		metas = append(metas, m)
	}
	return metas, rows.Err()
}

// GetUserByHandle resolves the user (and their credentials) from a WebAuthn user
// handle — the raw 16-byte UUID baked into the credential at registration.
func GetUserByHandle(ctx context.Context, pool *pgxpool.Pool, handle []byte) (*User, []webauthn.Credential, error) {
	id, err := uuid.FromBytes(handle)
	if err != nil {
		return nil, nil, err
	}
	return getUserWithCreds(ctx, pool, id.String())
}

// GetCredentialUser resolves the owning user (and their credentials) from a raw
// credential ID — the fallback when no user handle is present in an assertion.
func GetCredentialUser(ctx context.Context, pool *pgxpool.Pool, credentialID []byte) (*User, []webauthn.Credential, error) {
	var userID string
	err := pool.QueryRow(ctx,
		`SELECT user_id::text FROM webauthn_credentials WHERE credential_id = $1`, credentialID).Scan(&userID)
	if err != nil {
		return nil, nil, err
	}
	return getUserWithCreds(ctx, pool, userID)
}

func getUserWithCreds(ctx context.Context, pool *pgxpool.Pool, userID string) (*User, []webauthn.Credential, error) {
	user, err := GetUserByID(ctx, pool, userID)
	if err != nil {
		return nil, nil, err
	}
	creds, err := ListCredentialsByUser(ctx, pool, userID)
	if err != nil {
		return nil, nil, err
	}
	return user, creds, nil
}

// UpdateCredentialOnLogin persists the post-assertion authenticator state: the
// advanced sign counter, last-used time, and any clone/backup-state change.
func UpdateCredentialOnLogin(ctx context.Context, pool *pgxpool.Pool, c *webauthn.Credential) error {
	_, err := pool.Exec(ctx,
		`UPDATE webauthn_credentials
		 SET sign_count = $1, clone_warning = $2, backup_state = $3, last_used_at = now()
		 WHERE credential_id = $4`,
		int64(c.Authenticator.SignCount), c.Authenticator.CloneWarning, c.Flags.BackupState, c.ID)
	return err
}

// DeleteCredential removes a credential, scoped to its owner so a cross-user id
// affects no rows (returns 0 → caller responds 404, preventing IDOR).
func DeleteCredential(ctx context.Context, pool *pgxpool.Pool, userID, rowID string) (int64, error) {
	tag, err := pool.Exec(ctx,
		`DELETE FROM webauthn_credentials WHERE id = $1 AND user_id = $2`, rowID, userID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// CountCredentials returns how many passkeys a user has enrolled.
func CountCredentials(ctx context.Context, pool *pgxpool.Pool, userID string) (int, error) {
	var n int
	err := pool.QueryRow(ctx, `SELECT count(*) FROM webauthn_credentials WHERE user_id = $1`, userID).Scan(&n)
	return n, err
}

package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// totpEncPrefix marks a TOTP secret that is encrypted at rest; a value without
// it is treated as legacy plaintext.
const totpEncPrefix = "enc:v1:"

// resolveTOTPKey returns the 32-byte key used to encrypt TOTP secrets at rest,
// or nil when no stable key is available (in which case secrets are stored as
// plaintext — the legacy/quick-start behavior). Preference:
//  1. TOTP_ENC_KEY (base64 of 32 bytes) — a dedicated key the operator manages.
//  2. Derived from the JWT secret via HKDF, but only when that secret is stable
//     (explicitly set, or DEV_MODE). An ephemeral JWT secret changes on every
//     restart and would make stored secrets undecryptable, so it is not used.
func resolveTOTPKey(totpEncKey, jwtSecret string, jwtStable bool) ([]byte, error) {
	if totpEncKey != "" {
		k, err := base64.StdEncoding.DecodeString(totpEncKey)
		if err != nil {
			k, err = base64.RawURLEncoding.DecodeString(totpEncKey)
		}
		if err != nil || len(k) != 32 {
			return nil, errors.New("TOTP_ENC_KEY must be base64 of exactly 32 bytes")
		}
		return k, nil
	}
	if !jwtStable {
		return nil, nil
	}
	return hkdf.Key(sha256.New, []byte(jwtSecret), nil, "blackhaul-totp-enc-v1", 32)
}

// encryptTOTPSecret encrypts plaintext with AES-256-GCM and returns a prefixed,
// base64 value. With a nil key (encryption disabled) or empty input it returns
// the input unchanged.
func encryptTOTPSecret(key []byte, plaintext string) (string, error) {
	if key == nil || plaintext == "" {
		return plaintext, nil
	}
	gcm, err := newTOTPGCM(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return totpEncPrefix + base64.StdEncoding.EncodeToString(ct), nil
}

// decryptTOTPSecret reverses encryptTOTPSecret. A value without the prefix is
// treated as legacy plaintext and returned as-is.
func decryptTOTPSecret(key []byte, stored string) (string, error) {
	if !strings.HasPrefix(stored, totpEncPrefix) {
		return stored, nil // legacy plaintext (or empty)
	}
	if key == nil {
		return "", errors.New("encrypted TOTP secret but no encryption key configured")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, totpEncPrefix))
	if err != nil {
		return "", err
	}
	gcm, err := newTOTPGCM(key)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, ct := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

func newTOTPGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// backfillTOTPEncryption encrypts any plaintext TOTP secrets left in the
// database once a key is available (mirrors the daemon-token backfill). No-op
// when encryption is disabled.
func backfillTOTPEncryption(ctx context.Context, pool *pgxpool.Pool, key []byte) error {
	if key == nil {
		return nil
	}
	rows, err := pool.Query(ctx,
		`SELECT id, totp_secret FROM users WHERE totp_secret IS NOT NULL AND totp_secret <> '' AND totp_secret NOT LIKE 'enc:v1:%'`)
	if err != nil {
		return err
	}
	type rec struct{ id, secret string }
	var todo []rec
	for rows.Next() {
		var r rec
		if err := rows.Scan(&r.id, &r.secret); err != nil {
			rows.Close()
			return err
		}
		todo = append(todo, r)
	}
	rows.Close()
	for _, r := range todo {
		enc, err := encryptTOTPSecret(key, r.secret)
		if err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, `UPDATE users SET totp_secret = $1 WHERE id = $2`, enc, r.id); err != nil {
			return err
		}
	}
	if len(todo) > 0 {
		slog.Info("encrypted plaintext TOTP secrets at rest", "count", len(todo))
	}
	return nil
}

package main

import (
	"context"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID           string
	Username     string
	Email        string
	PasswordHash string
	TotpSecret   string
	TokenVersion int
}

type SessionClaims struct {
	jwt.RegisteredClaims
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Purpose  string `json:"purpose,omitempty"` // "totp_challenge" for login 2FA step
	Ver      int    `json:"ver,omitempty"`     // user's token_version at issue time; mismatch = revoked
}

func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// dummyPasswordHash is compared against when the username is unknown, so the
// login path spends the same bcrypt time whether or not the user exists (no
// timing-based user enumeration). Computed once at startup.
var dummyPasswordHash, _ = bcrypt.GenerateFromPassword([]byte("blackhaul-no-such-user"), bcrypt.DefaultCost)

func CreateUser(ctx context.Context, pool *pgxpool.Pool, username, password string) (*User, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	var id string
	err = pool.QueryRow(ctx,
		`INSERT INTO users (username, password_hash) VALUES ($1, $2) RETURNING id`,
		username, hash,
	).Scan(&id)
	if err != nil {
		return nil, err
	}
	return &User{ID: id, Username: username, PasswordHash: hash}, nil
}

func CreateUserWithTOTP(ctx context.Context, pool *pgxpool.Pool, username, password, totpSecret string) (*User, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	var id string
	err = pool.QueryRow(ctx,
		`INSERT INTO users (username, password_hash, totp_secret) VALUES ($1, $2, $3) RETURNING id`,
		username, hash, totpSecret,
	).Scan(&id)
	if err != nil {
		return nil, err
	}
	return &User{ID: id, Username: username, PasswordHash: hash, TotpSecret: totpSecret}, nil
}

func HasAnyUser(ctx context.Context, pool *pgxpool.Pool) (bool, error) {
	var n int
	err := pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func GetUserByUsername(ctx context.Context, pool *pgxpool.Pool, username string) (*User, error) {
	var u User
	err := pool.QueryRow(ctx,
		`SELECT id, username, COALESCE(email, ''), COALESCE(password_hash, ''), COALESCE(totp_secret, ''), token_version FROM users WHERE username = $1`,
		username,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.TotpSecret, &u.TokenVersion)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func GetUserByID(ctx context.Context, pool *pgxpool.Pool, userID string) (*User, error) {
	var u User
	err := pool.QueryRow(ctx,
		`SELECT id, username, COALESCE(email, ''), COALESCE(password_hash, ''), COALESCE(totp_secret, ''), token_version FROM users WHERE id = $1`,
		userID,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.TotpSecret, &u.TokenVersion)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetTokenVersion returns the user's current token_version (for session validation).
func GetTokenVersion(ctx context.Context, pool *pgxpool.Pool, userID string) (int, error) {
	var v int
	err := pool.QueryRow(ctx, `SELECT token_version FROM users WHERE id = $1`, userID).Scan(&v)
	return v, err
}

// BumpTokenVersion invalidates all outstanding session tokens for the user.
func BumpTokenVersion(ctx context.Context, pool *pgxpool.Pool, userID string) error {
	_, err := pool.Exec(ctx, `UPDATE users SET token_version = token_version + 1 WHERE id = $1`, userID)
	return err
}

func IssueToken(userID, username string, tokenVersion int, jwtSecret string, expiresIn time.Duration) (string, error) {
	claims := SessionClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiresIn)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		UserID:   userID,
		Username: username,
		Ver:      tokenVersion,
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString([]byte(jwtSecret))
}

const loginTokenExpiry = 5 * time.Minute

// IssueLoginToken issues a short-lived token for the TOTP verification step (purpose: totp_challenge).
func IssueLoginToken(userID, username, jwtSecret string) (string, error) {
	claims := SessionClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(loginTokenExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		UserID:   userID,
		Username: username,
		Purpose:  "totp_challenge",
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString([]byte(jwtSecret))
}

func ValidateToken(tokenString, jwtSecret string) (*SessionClaims, error) {
	t, err := jwt.ParseWithClaims(tokenString, &SessionClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(jwtSecret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := t.Claims.(*SessionClaims)
	if !ok || !t.Valid {
		return nil, jwt.ErrTokenInvalidClaims
	}
	return claims, nil
}

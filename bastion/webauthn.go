package main

import (
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
)

// newWebAuthn builds the WebAuthn relying party from config. Returns nil when
// RP_ID is unset (passkey features disabled). Resident keys + user verification
// are required so that discoverable (usernameless) login works and every login
// is a true second factor.
func newWebAuthn(cfg Config) (*webauthn.WebAuthn, error) {
	if cfg.RPID == "" {
		return nil, nil
	}
	return webauthn.New(&webauthn.Config{
		RPID:          cfg.RPID,
		RPDisplayName: cfg.RPDisplayName,
		RPOrigins:     cfg.RPOrigins,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:        protocol.ResidentKeyRequirementRequired,
			RequireResidentKey: protocol.ResidentKeyRequired(),
			UserVerification:   protocol.VerificationRequired,
		},
	})
}

// webauthnUser adapts our User + its stored credentials to the webauthn.User
// interface the library consumes during ceremonies.
type webauthnUser struct {
	user  *User
	creds []webauthn.Credential
}

// WebAuthnID is the stable user handle. We reuse the user's UUID as raw 16
// bytes — opaque, stable, and not PII. Always encode/decode this way (never the
// 36-char string) so discoverable login can recover the user from the handle.
func (u *webauthnUser) WebAuthnID() []byte {
	id, err := uuid.Parse(u.user.ID)
	if err != nil {
		return nil
	}
	b, _ := id.MarshalBinary()
	return b
}

func (u *webauthnUser) WebAuthnName() string                       { return u.user.Username }
func (u *webauthnUser) WebAuthnDisplayName() string                { return u.user.Username }
func (u *webauthnUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

// credentialDescriptors returns the exclude/allow descriptors for this user's
// existing credentials (used to stop double-registering one authenticator).
func (u *webauthnUser) credentialDescriptors() []protocol.CredentialDescriptor {
	ds := make([]protocol.CredentialDescriptor, 0, len(u.creds))
	for i := range u.creds {
		ds = append(ds, u.creds[i].Descriptor())
	}
	return ds
}

// transportsToStrings / stringsToTransports convert between the library's typed
// transport slice and the plain []string we persist in a Postgres text[].
func transportsToStrings(ts []protocol.AuthenticatorTransport) []string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, string(t))
	}
	return out
}

func stringsToTransports(ss []string) []protocol.AuthenticatorTransport {
	out := make([]protocol.AuthenticatorTransport, 0, len(ss))
	for _, s := range ss {
		out = append(out, protocol.AuthenticatorTransport(s))
	}
	return out
}

# Security Policy

Blackbox brokers access to files on your machines, so security reports take
priority over all other work.

## Reporting a vulnerability

**Please do not open a public issue for security problems.**

- **Preferred:** [GitHub private vulnerability reporting](https://github.com/zdods/blackbox/security/advisories/new)
  (Security → Report a vulnerability).
- **Email:** zach@zdods.com with the subject `blackbox security`.

Include what you can of: affected component (bastion / daemon / web console),
version (`--version` or image tag), deployment method (Docker image, compose,
source build), reproduction steps or proof of concept, and your assessment of
the impact.

**What to expect:** acknowledgement within 72 hours, a status update at least
weekly while we investigate, and coordinated disclosure — we'll agree on a
timeline with you before details are published. Credit in the release notes
and changelog if you want it.

## Supported versions

Blackbox is pre-1.0: **only the latest release** receives security fixes.
If you can, confirm the issue reproduces on the
[latest release](https://github.com/zdods/blackbox/releases/latest) before
reporting — but when in doubt, report anyway.

## Scope

In scope:

- The bastion server (auth, session handling, file-proxy API, daemon hub)
- The daemon (path traversal, token handling, WebSocket client)
- The web console
- Release artifacts and the pipeline that produces them (checksums,
  install script, published images)

Out of scope:

- Issues that require an already-compromised host running the daemon or
  bastion
- Resource exhaustion from large transfers on an instance you operate
  (the bastion proxies all bytes by design)
- Vulnerabilities in third-party dependencies with no demonstrated impact
  on blackbox — report upstream, but feel free to tell us too so we can
  bump the dependency

## Hardening your deployment

- Run the bastion behind TLS (terminate at a reverse proxy, or set
  `TLS_CERT_FILE`/`TLS_KEY_FILE`) and use `wss://` daemon URLs.
- Set a stable `JWT_SECRET` (`openssl rand -base64 32`); without one the
  server generates an ephemeral secret and sessions reset on restart.
- Keep Postgres unexposed (the provided compose binds it to `127.0.0.1`).

Internal security design and audit notes live in
[docs/security-notes.md](docs/security-notes.md).

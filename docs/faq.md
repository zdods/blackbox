# FAQ & troubleshooting

For how the pieces fit together, see [architecture.md](architecture.md). For
running behind TLS, see [deployment.md](deployment.md).

## FAQ

### Is my data safe? Where do my files live?

Your files stay **on your machines**. The bastion is a relay: it proxies bytes
between the console and a daemon and **never writes file content to disk or to
the database**. Postgres holds only your account and the list of registered
daemons — no files, no directory listings.

What protects access:

- One account, **bcrypt** password, **mandatory TOTP (2FA)**. The 2FA secret is
  **encrypted at rest** (set `TOTP_ENC_KEY`, or it's derived from a stable
  `JWT_SECRET`).
- Sessions are signed JWTs in **httpOnly cookies** (not readable by JS, never in
  localStorage); logging out **revokes all sessions** at once.
- Daemon tokens are stored **hashed** (SHA-256); the plaintext exists only in
  the daemon's `0600` config file on the host. Each daemon is scoped to its
  owning user.
- Each daemon is scoped to **one directory** (its hosted path) and rejects path
  traversal outside it — including via symlinks that point out of the root.

### Does the server see my file contents?

Yes — while a transfer is in flight, the bytes pass **through** the bastion
process (that is what a relay does). They are not stored, but they are visible
to the server in transit. So:

- Always run the bastion behind **TLS** (`wss://` for daemons). See
  [deployment.md](deployment.md).
- Run the bastion somewhere you trust. Self-hosting means you control it; on the
  future hosted offering, content transits but is never stored.

This is the honest tradeoff of "browse my machines from anywhere with zero
network config." If you need the server to never see plaintext, blackhaul's
relay model isn't the right tool.

### Do I have to open ports on my machines?

**No.** Daemons make an **outbound** WebSocket connection to the bastion. Your
laptop/NAS/server needs no inbound ports and no firewall changes — it just needs
to reach the bastion (outbound 443 for a TLS deployment). Only the bastion is
exposed to the internet.

### Can I add more than one user / share with someone?

Not today — blackhaul is **single-user by design**. One account reaches all
registered daemons. Multi-user and share links are on the roadmap but gated on
demand.

### What happens with very large files?

Transfers are memory-bounded end to end. The console chunks uploads at 5 MB; the
bastion never buffers more than ~6 MB of a request in RAM; downloads stream
straight to disk in your browser. A 50 GB file moves in 5 MB chunks, not one
50 GB gulp. (See the v0.5.0 entry in the [CHANGELOG](../CHANGELOG.md).)

### How do I back up?

Back up the **Postgres database** — it's tiny and contains only your user and
daemon registrations (no files). Your actual files are already on your machines;
back those up however you already do. Re-registering a daemon just means issuing
a new token.

### I lost my password / 2FA device. How do I reset?

There is no email-based recovery (single-user, no mail server). Recover by
editing the database directly: clear or update the row in the `users` table
(e.g. reset `totp_secret` to re-enroll, or remove the user to re-run
registration). Keep your TOTP recovery in your password manager.

## Troubleshooting

### The daemon won't connect / `websocket: bad handshake`

Almost always the reverse proxy isn't forwarding the **WebSocket upgrade** on
`/ws/daemon`. Most proxies don't by default. Fix the proxy config (nginx/Caddy/
Traefik examples in [deployment.md](deployment.md)). The daemon logs diagnose
the dial failure.

### `ws://` fails against an HTTPS server

WebSocket dials **don't follow redirects**, so `ws://` against an HTTP→HTTPS
redirect just fails. Use **`wss://`** once TLS is terminated:

```bash
./blackhaul-daemon --bastion-url=wss://blackhaul.example.com/ws/daemon
```

### A host shows "offline" in the console

The daemon isn't connected. Check, on the host:

- the daemon process is running (`systemctl status blackhaul-daemon`,
  `launchctl list | grep blackhaul`, or the Windows service);
- the **bastion URL** is reachable from the host (outbound 443/`wss://`);
- the **token** matches the one issued for that host (re-add the host to mint a
  fresh token if unsure);
- logs: `journalctl -u blackhaul-daemon -f` (Linux),
  `~/Library/Logs/blackhaul-daemon.log` (macOS).

### The console shows "host not connected" (503) when browsing

Same cause as above — the bastion has no live WebSocket for that daemon. The
file API returns `503` until the daemon reconnects.

### I'm logged out after every server restart

`JWT_SECRET` isn't set, so the bastion generates a new **ephemeral** key each
start and old sessions stop validating. Set a stable secret in production (it
must be **at least 32 bytes**, or the bastion refuses to start):

```bash
JWT_SECRET=$(openssl rand -base64 32)
```

If you derive the TOTP encryption key from `JWT_SECRET` (i.e. no `TOTP_ENC_KEY`),
**don't rotate `JWT_SECRET` after enrolling 2FA** — the stored TOTP secret would
become undecryptable. Set a dedicated `TOTP_ENC_KEY` to rotate them independently.

### Locked out of login / 2FA

- Login is rate-limited to **10/min per IP**. Behind a proxy, set
  `TRUST_PROXY=1` so the limit keys on the real client IP, not the proxy's.
- TOTP **locks for 15 minutes after 5 failed codes**. Wait it out, and check
  your authenticator's clock is in sync.

### Upload rejected with `413`

Means a single, non-chunked upload exceeded the 6 MB cap. The console chunks
automatically, so you'll only see this from a custom API client doing a
single-shot `PUT` of a large file — switch it to the chunked upload protocol
(see [architecture.md](architecture.md#uploads)).

### After upgrading, my Docker data volume looks empty

As of v0.5.0 the compose project name is pinned to `blackhaul`, so the data
volume is named `blackhaul-*` regardless of your checkout directory. An older
deployment's volume had a different name. To keep the old data, set
`COMPOSE_PROJECT_NAME` to your previous project name, or copy the old volume's
contents over before removing it.

## Still stuck?

- Bug or question: [open an issue](https://github.com/zdods/blackhaul/issues).
- Security problem: **don't** open a public issue — see
  [SECURITY.md](../SECURITY.md).

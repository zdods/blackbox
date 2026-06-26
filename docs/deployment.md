# Deploying behind a reverse proxy (TLS)

The bastion serves three kinds of traffic on one port (default `:8080`):

- the web console (static files + `/api/*`)
- the daemon WebSocket at **`/ws/daemon`**
- `/healthz`

Everything is ordinary HTTP except `/ws/daemon`, which needs a **WebSocket
upgrade**. Most reverse proxies do not forward upgrades by default — if the
daemon logs `websocket: bad handshake`, that is almost always the proxy.

Once TLS terminates at the proxy, daemons must use a **`wss://`** URL:

```bash
./blackhaul-daemon --bastion-url=wss://blackhaul.example.com/ws/daemon
```

(`ws://` against an HTTP→HTTPS redirect fails — WebSocket dials do not follow
redirects. The daemon log tells you when this is the case.)

## nginx / OpenResty

```nginx
# Upgrade WebSocket connections when the client asks for it.
map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
}

server {
    listen 443 ssl;
    server_name blackhaul.example.com;

    # ssl_certificate     /path/fullchain.pem;
    # ssl_certificate_key /path/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;

        # WebSocket upgrade (required for /ws/daemon; harmless elsewhere)
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;

        # Daemon connections are long-lived; the 60s default would cut
        # idle daemons off. Uploads/downloads also stream through here.
        proxy_read_timeout 1h;
        proxy_send_timeout 1h;

        # Don't buffer streamed file transfers to disk.
        proxy_request_buffering off;
        proxy_buffering off;

        # Allow uploads larger than the 1m default.
        client_max_body_size 512m;
    }
}

server {
    listen 80;
    server_name blackhaul.example.com;
    return 301 https://$host$request_uri;
}
```

The three essentials: `proxy_http_version 1.1`, `Upgrade`/`Connection`
headers, and generous read timeouts. Everything else is tuning.

## Caddy

Caddy proxies WebSockets and provisions Let's Encrypt automatically — no
special configuration:

```caddyfile
blackhaul.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

## Traefik (Docker labels)

```yaml
labels:
  - traefik.enable=true
  - traefik.http.routers.blackhaul.rule=Host(`blackhaul.example.com`)
  - traefik.http.routers.blackhaul.entrypoints=websecure
  - traefik.http.routers.blackhaul.tls.certresolver=letsencrypt
  - traefik.http.services.blackhaul.loadbalancer.server.port=8080
```

Traefik forwards WebSocket upgrades by default.

## Bastion settings behind a proxy

| Env var | Why |
|---|---|
| `TRUST_PROXY=1` | Rate limiting keys on the client IP. Behind a proxy every request comes from the proxy's IP — this trusts the right-most `X-Forwarded-For` hop instead. Set it **only** when the proxy appends/overwrites that header (the configs above do). |
| `JWT_SECRET` | Set a stable secret (`openssl rand -base64 32`, **≥32 bytes** or the server won't start) or sessions reset on every restart. |
| `COOKIE_SECURE=1` | Sets the `Secure` flag on the session cookie. Needed when TLS terminates at the proxy and the bastion speaks plain HTTP. |
| `TOTP_ENC_KEY` | Encrypts 2FA secrets at rest with a dedicated key (base64 of 32 bytes); otherwise derived from `JWT_SECRET`. |
| `AUTH_MODE` | `password` (default), `passkey`, or `both`. Selects the sign-in methods offered. `passkey` requires `RP_ID`; `both` shows passkey + password side by side (and degrades to password-only if `RP_ID` is unset). Passkey enrollment/management works in every mode, so you can add a passkey in password mode and then switch. |
| `RP_ID` | WebAuthn Relying Party ID for passkeys — the registrable **domain only**, no scheme/port (e.g. `blackhaul.example.com`). Must equal the public hostname the browser uses (the same host the proxy serves). Use `localhost` for local HTTP dev. |
| `RP_ORIGINS` | Comma-separated full origins allowed for passkey ceremonies (scheme + host + non-default port). Defaults to `https://<RP_ID>`. Set explicitly when the public origin differs (e.g. a non-443 port). |
| `RP_DISPLAY_NAME` | Relying-party name shown by authenticators during a passkey prompt. Defaults to `Blackhaul`. |

TLS between the proxy and the bastion on the same host is unnecessary; if
the proxy is on a different machine, either run the link over a private
network or give the bastion its own certificate (`TLS_CERT_FILE`/`TLS_KEY_FILE`).

## Verifying

```bash
# Should answer 101 Switching Protocols (anything else is the proxy):
curl -si https://blackhaul.example.com/ws/daemon \
  -H "Connection: Upgrade" -H "Upgrade: websocket" \
  -H "Sec-WebSocket-Version: 13" -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" | head -1

# Health endpoint through the proxy:
curl -s https://blackhaul.example.com/healthz
```

| Symptom (daemon log) | Cause |
|---|---|
| `HTTP 301 redirect … use a wss:// URL` | URL is `ws://` but the site redirects to HTTPS |
| `HTTP 400 — … Upgrade and Connection headers` | Proxy not forwarding the WebSocket upgrade |
| `HTTP 502` | Proxy can't reach the bastion (wrong upstream/port) |
| Daemon disconnects every ~60s | Proxy read timeout too low for idle connections |

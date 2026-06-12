# CLAUDE.md

Self-hosted, single-user cloud storage. Monorepo with Go backend, SvelteKit frontend, and cross-platform daemon.

## Layout

- `bastion/` — Go server (auth, daemon hub, file-proxy API, serves the SvelteKit console)
- `daemon/` — Go binary (WebSocket client, file handlers; Linux/macOS/Windows)
- `web/` — SvelteKit frontend (login, dashboard, file browser)
- `pkg/` — shared Go message types (daemon ↔ server protocol)

## Tech stack

- **Go 1.22+** with stdlib only (`net/http`, `ServeMux`) — no external web frameworks
- **SvelteKit 5** with `adapter-static` (static site generation)
- **PostgreSQL** for persistence
- **WebSockets** (`gorilla/websocket`) for daemon ↔ server communication

## Go conventions

- Use `net/http` and the `ServeMux` introduced in Go 1.22 for all routing
- No external web frameworks — stdlib only
- Handler signature: `func(w http.ResponseWriter, r *http.Request)`
- Return proper HTTP status codes with JSON responses
- Use custom error types when beneficial
- Validate all input at API boundaries
- Use middleware for cross-cutting concerns (logging, auth, rate limiting)
- Use the standard `log` package for logging
- Leverage goroutines and channels when beneficial for performance

## SvelteKit conventions

- File-based routing in `src/routes/`
- Use `+page.svelte`, `+layout.svelte`, `+error.svelte`, and `+page.js`/`+page.server.js` conventions
- Scoped styles with `<style>` tags in `.svelte` files
- Use Svelte stores for global state management
- Use form actions for server-side form handling
- Use `load` functions for data fetching
- Progressive enhancement for forms (JavaScript-optional submissions)

## HTML & CSS conventions

- Semantic HTML (`<header>`, `<main>`, `<footer>`, `<article>`, `<section>`, `<nav>`)
- Use `<button>` for clickable elements, `<a>` for links with `href`
- BEM methodology for CSS class naming
- Flexbox and Grid for layout
- `rem`/`em` units for typography
- CSS variables for theming
- Mobile-first responsive design with media queries
- ARIA roles and attributes for accessibility
- No `!important` — use specificity instead

## Commit messages

Use Conventional Commits: `<type>(<scope>): <short description>`

**Types:** `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`, `perf`, `ci`, `build`

**Scopes:** `dev`, `daemon`, `bastion`, `web`, `pkg`

- Imperative, lowercase after the colon, no trailing period
- Max ~72 characters for subject line
- Optional body (wrap at 72 chars) explaining what and why

## Changelog

`CHANGELOG.md` follows [keep-a-changelog](https://keepachangelog.com). When a
change is **user-visible** (feature, fix, security, behavior change), add an
entry under `[Unreleased]` in the same commit/PR. Internal-only changes
(refactors, CI, deps, docs) don't get entries. At release time the
`cut-release` skill (`.claude/skills/cut-release/`) rolls `[Unreleased]` into
the new version section before tagging — never tag with stale changelog.

## Dev workflow

```bash
# Docker (hot reload)
make dev          # or: docker compose up --build --watch

# Docker (one-shot)
make up           # or: docker compose up --build

# Local dev (no Docker)
cd web && npm install && npm run build && cd ..
export DATABASE_URL=postgres://postgres@localhost:5432/blackhaul?sslmode=disable
export STATIC_DIR=web/build
go run ./bastion

# Build daemon
go build -o blackhaul-daemon ./daemon

# Run daemon (interactive first-time setup; saves config to ~/.blackhaul-daemon)
./blackhaul-daemon

# Run daemon with explicit values (flags > env vars > config file > interactive)
./blackhaul-daemon --bastion-url=ws://localhost:8080/ws/daemon --token=TOKEN --hosted-path=~/files
BLACKHAUL_TOKEN=TOKEN BLACKHAUL_HOSTED_PATH=~/files ./blackhaul-daemon
```

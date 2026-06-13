# Contributing to blackhaul

Thanks for your interest! Issues and pull requests are welcome.

## Dev setup

Prerequisites: **Go 1.25+**, **Node 22** (see `.nvmrc`), **Docker** (or a
local Postgres).

```bash
# Docker with hot reload (bastion + Postgres + web watch)
make dev

# Or run locally without Docker
cd web && npm install && npm run build && cd ..
export DATABASE_URL=postgres://postgres@localhost:5432/blackhaul?sslmode=disable
export STATIC_DIR=web/build
go run ./bastion

# Build and run the daemon
make build-daemon
./blackhaul-daemon
```

See the [README](README.md) for the full picture (daemon setup, TLS, service
units) and [CLAUDE.md](CLAUDE.md) for layout and code conventions.

## Tests

```bash
go test ./bastion/ ./daemon/ ./pkg/...   # or: make test
```

CI runs the Go tests and builds the web app on every PR. Changes to
security-relevant code (auth, sessions, path handling, rate limiting) should
come with tests.

For a full end-to-end check against a real running stack (bastion + Postgres +
a real daemon, exercising every feature by hand), run the smoke test — handy
before cutting a release:

```bash
bash .claude/skills/smoke-test/smoke-test.sh   # ends with "N passed, 0 failed"
```

## Commit messages

Conventional Commits: `<type>(<scope>): <short description>`

- **Types:** `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`,
  `perf`, `ci`, `build`
- **Scopes:** `dev`, `daemon`, `bastion`, `web`, `pkg`
- Imperative, lowercase after the colon, no trailing period, subject ≤ ~72
  chars. Optional body (wrapped at 72) explaining what and why.

## Changelog

User-visible changes (features, fixes, security, behavior changes) get an
entry under `[Unreleased]` in [CHANGELOG.md](CHANGELOG.md)
([keep-a-changelog](https://keepachangelog.com) format) in the same PR.
Internal-only changes (refactors, CI, deps) don't need one.

## Pull requests

- Keep PRs small and focused — one logical change per PR.
- CI must pass; include tests for behavior changes.
- Fill in the PR template (it has a short checklist).

## Security issues

**Do not open a public issue.** See [SECURITY.md](SECURITY.md) for private
reporting.

## Releases (maintainers)

Releases are semver tags (`vX.Y.Z`). Pushing a tag triggers
`.github/workflows/release.yml`: the multi-arch bastion image to GHCR, and
daemon binaries + checksums to the GitHub Release via GoReleaser. The
`[Unreleased]` changelog section is rolled into the new version **before**
tagging — the `cut-release` skill in `.claude/skills/` walks through the
whole flow.

---
name: cut-release
description: Propose and cut a new release. Reviews all changes between the latest semver tag and main, decides whether they warrant a release, proposes the next version per semver + conventional commits, rolls the CHANGELOG, and on approval tags and pushes (which triggers the image + daemon-binary release workflow). Use when the user asks to cut/prepare/propose a release or asks "should we release?"
---

# Cut a release

Releases are driven by semver git tags (`vX.Y.Z`). Pushing a tag triggers
`.github/workflows/release.yml`, which:

- builds and pushes the multi-arch bastion image to
  `ghcr.io/<owner>/blackbox-bastion` with SBOM + provenance, and
- runs **GoReleaser** (`.goreleaser.yaml`): daemon binaries for
  linux/darwin/windows × amd64/arm64, archives + `checksums.txt` attached to
  a **GitHub Release it creates itself**, and a Homebrew cask pushed to
  `zdods/homebrew-tap` (auto-skipped when `TAP_GITHUB_TOKEN` is unset).

## 1. Establish the range

```bash
git fetch --tags origin
LATEST=$(git tag --list 'v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname | head -1)
git log --oneline ${LATEST:+$LATEST..}origin/main
git diff --stat ${LATEST:+$LATEST..}origin/main
```

- No tags yet → this is the first release; treat the whole history as the range.
- Always compare against `origin/main`, not the local branch — releases cut
  what's actually pushed. If local main is ahead of origin, say so and stop;
  the user must push (or ask you to) first.

## 2. Decide whether it warrants a release

Read the commits (and diffs where the message is vague — messages are claims,
diffs are truth). Classify each as: breaking change, feature (`feat`),
fix (`fix`), security hardening, or chore/docs/CI-only.

- Only chores, docs, CI, or dependency bumps with no user-visible effect →
  recommend **no release**; report what's pending and stop.
- Anything user-visible (features, fixes, security, behavior changes) →
  propose a release.

## 3. Propose the version

Conventional commits → semver, with pre-1.0 conventions (0.x: breaking
changes bump **minor**, everything else bumps **patch**):

| Changes in range | 0.x bump | ≥1.0 bump |
|---|---|---|
| Breaking change (`feat!`, `BREAKING CHANGE:`, removed/renamed config or API) | minor | major |
| New feature (`feat:`) | minor (or patch if trivial) | minor |
| Fixes / security / perf only | patch | patch |

Present to the user:
- proposed version (and the rule that produced it)
- a short, grouped changelog (Breaking / Features / Fixes / Security)
- anything risky in the range (migrations, config changes, defaults)

**Stop and get explicit approval before tagging.** Never tag or push
without it.

## 4. Roll the CHANGELOG (after approval, before tagging)

`CHANGELOG.md` (keep-a-changelog) must be rolled **before** the tag so the
release commit ships with an accurate changelog:

1. Cross-check `[Unreleased]` against the commit range — add any
   user-visible change that's missing (contributors sometimes forget;
   the PR template asks but doesn't enforce).
2. Rename `[Unreleased]` to `[X.Y.Z] - YYYY-MM-DD` and add a fresh, empty
   `[Unreleased]` section above it.
3. Update the link references at the bottom (`[Unreleased]: …compare/vX.Y.Z...HEAD`
   and the new version's compare/tag link).
4. Commit and push:

```bash
git add CHANGELOG.md
git commit -m "docs: roll changelog for vX.Y.Z"
git push origin main
```

The tag goes on this commit.

## 5. Cut it

```bash
git tag -a vX.Y.Z -m "vX.Y.Z" <sha-of-changelog-commit-on-origin-main>
git push origin vX.Y.Z
```

Then watch the workflow and verify **both** jobs' artifacts:

```bash
gh run watch --exit-status $(gh run list --workflow=release.yml --limit 1 --json databaseId --jq '.[0].databaseId')

# Image landed
docker pull ghcr.io/<owner>/blackbox-bastion:X.Y.Z   # or inspect via gh api

# Release assets: 4 tar.gz + 2 zip + checksums.txt
gh release view vX.Y.Z --json assets --jq '.assets[].name'

# Cask updated (only if TAP_GITHUB_TOKEN is configured)
gh api repos/<owner>/homebrew-tap/contents/Casks/blackbox-daemon.rb --jq '.size' 2>/dev/null
```

If the workflow fails, report the failure and do NOT delete/re-push the tag
until the user decides; a moved tag breaks anyone who already fetched it.

## 6. Release notes

GoReleaser **creates the GitHub Release itself** with commit-grouped notes
and a footer (image pull + installer one-liner) — do **not** `gh release
create`. Offer to polish the generated notes instead, e.g. prepend the
curated changelog section from step 4:

```bash
gh release view vX.Y.Z --json body --jq .body   # inspect
gh release edit vX.Y.Z --notes "<improved notes>"
```

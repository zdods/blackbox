---
name: cut-release
description: Propose and cut a new release. Reviews all changes between the latest semver tag and main, decides whether they warrant a release, proposes the next version per semver + conventional commits, and on approval tags and pushes (which triggers the Docker image release workflow). Use when the user asks to cut/prepare/propose a release or asks "should we release?"
---

# Cut a release

Releases are driven by semver git tags (`vX.Y.Z`). Pushing a tag triggers
`.github/workflows/release.yml`, which builds and pushes the multi-arch
bastion image to `ghcr.io/<owner>/blackbox-bastion` with SBOM + provenance.

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

## 4. Cut it (after approval)

```bash
git tag -a vX.Y.Z -m "vX.Y.Z" <sha-of-origin-main>
git push origin vX.Y.Z
```

Then watch the release workflow and confirm the image landed:

```bash
gh run watch --exit-status $(gh run list --workflow=release.yml --limit 1 --json databaseId --jq '.[0].databaseId')
docker pull ghcr.io/<owner>/blackbox-bastion:X.Y.Z   # spot-check (or inspect via gh api)
```

If the workflow fails, report the failure and do NOT delete/re-push the tag
until the user decides; a moved tag breaks anyone who already fetched it.

## 5. GitHub Release notes

Offer to create a GitHub Release with the grouped changelog from step 3:

```bash
gh release create vX.Y.Z --title "vX.Y.Z" --notes "<grouped changelog>"
```

Mention the published image (`ghcr.io/<owner>/blackbox-bastion:X.Y.Z`) in
the notes.

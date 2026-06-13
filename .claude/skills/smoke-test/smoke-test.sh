#!/usr/bin/env bash
# End-to-end smoke test: drives the real stack (bastion + Postgres + a real
# daemon binary) through every product feature, the same as a user would by
# hand. Builds the bastion from source, registers the single user with 2FA,
# logs in, manages a daemon, runs that daemon against a temp directory, and
# exercises file list/meta/download/upload(single + chunked)/delete plus the
# key security behaviors (wrong creds, IDOR, session revocation, upload caps).
#
# Usage: bash .claude/skills/smoke-test/smoke-test.sh
# Requires: docker, go, curl, jq. Run from the repo root.
set -uo pipefail

# --- config ----------------------------------------------------------------
BASE="http://localhost:8080"
WS="ws://localhost:8080/ws/daemon"
JAR="$(mktemp)"                 # session cookie jar
HOSTDIR="$(mktemp -d)"          # directory the daemon serves
DAEMON_BIN="$(mktemp -d)/blackhaul-daemon"
USERNAME="smoke"
PASSWORD="smoke-password-123"
export JWT_SECRET="smoke-test-jwt-secret-at-least-32-bytes-long"  # stable → TOTP encrypted at rest
DAEMON_PID=""
PASS=0
FAIL=0

cd "$(git rev-parse --show-toplevel)"
TOTP() { go run ./.claude/skills/smoke-test/totpgen "$1"; }

say()  { printf '\n\033[1;36m== %s\033[0m\n' "$*"; }
ok()   { PASS=$((PASS+1)); printf '  \033[32m✓\033[0m %s\n' "$*"; }
bad()  { FAIL=$((FAIL+1)); printf '  \033[31m✗ %s\033[0m\n' "$*"; }
check(){ # check <desc> <actual> <expected>
  if [ "$2" = "$3" ]; then ok "$1 ($2)"; else bad "$1: got '$2', want '$3'"; fi
}

cleanup() {
  say "Teardown"
  [ -n "$DAEMON_PID" ] && kill "$DAEMON_PID" 2>/dev/null
  docker compose -f docker-compose.yml -f docker-compose.dev.yml down -v >/dev/null 2>&1
  rm -rf "$JAR" "$HOSTDIR" "$(dirname "$DAEMON_BIN")"
  printf '\n\033[1m%d passed, %d failed\033[0m\n' "$PASS" "$FAIL"
}
trap cleanup EXIT

# curl helpers: status() prints HTTP code; body saved to $BODY
BODY="$(mktemp)"
status() { curl -s -o "$BODY" -w '%{http_code}' -b "$JAR" -c "$JAR" "$@"; }

# --- 0. bring up the stack -------------------------------------------------
say "Build + start the stack from source"
docker compose -f docker-compose.yml -f docker-compose.dev.yml up --build -d >/dev/null 2>&1 \
  || { bad "compose up failed"; exit 1; }
for i in $(seq 1 60); do
  [ "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/healthz")" = "200" ] && break
  sleep 1
done
check "healthz" "$(curl -s "$BASE/healthz" | jq -r .status)" "ok"

# --- 1. registration (single-user, mandatory 2FA) -------------------------
say "Registration"
check "registration open on fresh DB" "$(curl -s "$BASE/api/setup" | jq -r .registration_open)" "true"

code=$(status -X POST "$BASE/api/register/totp-setup" -H 'Content-Type: application/json' -d '{}')
check "totp-setup status" "$code" "200"
SECRET=$(jq -r .secret "$BODY")
[ -n "$SECRET" ] && ok "got TOTP secret + provisioning URI" || bad "no TOTP secret"
SETUP_ID=$(jq -r .setup_id "$BODY")

code=$(status -X POST "$BASE/api/register" -H 'Content-Type: application/json' \
  -d "{\"username\":\"$USERNAME\",\"password\":\"$PASSWORD\",\"totp_code\":\"$(TOTP "$SECRET")\",\"setup_id\":\"$SETUP_ID\"}")
check "register status" "$code" "201"
check "registration closed after first user" "$(curl -s "$BASE/api/setup" | jq -r .registration_open)" "false"

# --- 2. login (password + TOTP, with negative cases) ----------------------
say "Login"
code=$(status -X POST "$BASE/api/login" -H 'Content-Type: application/json' \
  -d "{\"username\":\"$USERNAME\",\"password\":\"wrong\"}")
check "login wrong password rejected" "$code" "401"

code=$(status -X POST "$BASE/api/login" -H 'Content-Type: application/json' \
  -d "{\"username\":\"$USERNAME\",\"password\":\"$PASSWORD\"}")
check "login password step" "$code" "200"
check "requires totp challenge" "$(jq -r .requires_totp "$BODY")" "true"
LOGIN_TOKEN=$(jq -r .login_token "$BODY")

code=$(status -X POST "$BASE/api/login/totp" -H 'Content-Type: application/json' \
  -d "{\"login_token\":\"$LOGIN_TOKEN\",\"code\":\"000000\"}")
check "login wrong TOTP rejected" "$code" "401"

# fresh challenge (the bad code may count toward lockout; get a new login_token)
status -X POST "$BASE/api/login" -H 'Content-Type: application/json' \
  -d "{\"username\":\"$USERNAME\",\"password\":\"$PASSWORD\"}" >/dev/null
LOGIN_TOKEN=$(jq -r .login_token "$BODY")
code=$(status -X POST "$BASE/api/login/totp" -H 'Content-Type: application/json' \
  -d "{\"login_token\":\"$LOGIN_TOKEN\",\"code\":\"$(TOTP "$SECRET")\"}")
check "login TOTP step (session issued)" "$code" "200"

code=$(status "$BASE/api/me")
check "me (authenticated)" "$(jq -r .username "$BODY")" "$USERNAME"
check "unauthenticated me rejected" "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/me")" "401"

# --- 3. daemon registration ------------------------------------------------
say "Daemon management"
code=$(status -X POST "$BASE/api/daemons" -H 'Content-Type: application/json' \
  -d '{"label":"smoke-host","hosted_path":"."}')
check "create daemon" "$code" "201"
DAEMON_ID=$(jq -r .id "$BODY")
TOKEN=$(jq -r .token "$BODY")
[ -n "$TOKEN" ] && ok "daemon token issued (shown once)" || bad "no daemon token"
check "daemon listed, disconnected" "$(status "$BASE/api/daemons" >/dev/null; jq -r '.[0].connected' "$BODY")" "false"

# --- 4. run the real daemon binary -----------------------------------------
say "Connect a real daemon"
go build -o "$DAEMON_BIN" ./daemon || bad "daemon build failed"
printf 'hello smoke test' > "$HOSTDIR/notes.txt"
mkdir -p "$HOSTDIR/sub"
printf 'nested' > "$HOSTDIR/sub/inner.txt"
head -c 6291456 /dev/urandom > "$HOSTDIR/big.bin"   # 6 MB (forces chunked download)
"$DAEMON_BIN" --bastion-url="$WS" --token="$TOKEN" --hosted-path="$HOSTDIR" >/tmp/smoke-daemon.log 2>&1 &
DAEMON_PID=$!
for i in $(seq 1 30); do
  status "$BASE/api/daemons" >/dev/null
  [ "$(jq -r '.[0].connected' "$BODY")" = "true" ] && break
  sleep 1
done
check "daemon connected" "$(status "$BASE/api/daemons" >/dev/null; jq -r '.[0].connected' "$BODY")" "true"
status "$BASE/api/daemons" >/dev/null
[ "$(jq -r '.[0].disk_total' "$BODY")" != "null" ] && ok "disk stats reported" || bad "no disk stats"

# --- 5. file operations ----------------------------------------------------
say "File operations"
code=$(status "$BASE/api/daemons/$DAEMON_ID/files?path=.")
check "list dir" "$code" "200"
names=$(jq -r '.[].name' "$BODY" | sort | tr '\n' ' ')
[ "$names" = "big.bin notes.txt sub " ] && ok "listing: $names" || bad "listing wrong: $names"

code=$(status "$BASE/api/daemons/$DAEMON_ID/meta?path=notes.txt")
check "meta size" "$(jq -r .size "$BODY")" "16"

# download (small)
curl -s -b "$JAR" "$BASE/api/daemons/$DAEMON_ID/files?path=notes.txt&download=1" -o "$BODY"
check "download (small) content" "$(cat "$BODY")" "hello smoke test"

# download (large, streaming chunked path) — verify byte-identical
curl -s -b "$JAR" "$BASE/api/daemons/$DAEMON_ID/files?path=big.bin&download=1" -o /tmp/smoke-big-dl.bin
check "download (large) size" "$(wc -c </tmp/smoke-big-dl.bin | tr -d ' ')" "6291456"
if cmp -s "$HOSTDIR/big.bin" /tmp/smoke-big-dl.bin; then ok "large download byte-identical"; else bad "large download differs"; fi

# upload (single-shot)
printf 'uploaded body' > /tmp/smoke-up.txt
code=$(status -X PUT "$BASE/api/daemons/$DAEMON_ID/files?path=uploaded.txt" --data-binary @/tmp/smoke-up.txt)
check "upload (single-shot)" "$code" "204"
check "uploaded file on disk" "$(cat "$HOSTDIR/uploaded.txt")" "uploaded body"

# upload (chunked, like the console: 5 MB chunks)
head -c 5500000 /dev/urandom > /tmp/smoke-chunked.bin
split -b 5242880 /tmp/smoke-chunked.bin /tmp/smoke-chunk-      # chunk-aa (5MB) + chunk-ab (rest)
UPID="smoke$(date +%s)"
i=0; total=$(ls /tmp/smoke-chunk-* | wc -l | tr -d ' ')
for part in /tmp/smoke-chunk-*; do
  code=$(status -X PUT "$BASE/api/daemons/$DAEMON_ID/files?path=chunked.bin&upload_id=$UPID&chunk_index=$i&total_chunks=$total" --data-binary @"$part")
  [ "$code" = "200" ] || bad "chunk $i upload status $code"
  i=$((i+1))
done
ok "chunked upload ($total chunks)"
if cmp -s /tmp/smoke-chunked.bin "$HOSTDIR/chunked.bin"; then ok "chunked upload assembled byte-identical"; else bad "chunked assembly differs"; fi

# upload cap: a >6 MB single-shot must be rejected (413)
head -c 7000000 /dev/urandom > /tmp/smoke-toobig.bin
code=$(status -X PUT "$BASE/api/daemons/$DAEMON_ID/files?path=toobig.bin" --data-binary @/tmp/smoke-toobig.bin)
check "oversize single-shot upload rejected" "$code" "413"

# delete
code=$(status -X DELETE "$BASE/api/daemons/$DAEMON_ID/files?path=uploaded.txt")
check "delete file" "$code" "204"
[ ! -f "$HOSTDIR/uploaded.txt" ] && ok "file gone from disk" || bad "file still on disk"

# --- 6. security behaviors -------------------------------------------------
say "Security behaviors"
check "IDOR: unowned daemon id → 404" \
  "$(status "$BASE/api/daemons/00000000-0000-0000-0000-000000000000/files?path=.")" "404"

# rename daemon
code=$(status -X PATCH "$BASE/api/daemons/$DAEMON_ID" -H 'Content-Type: application/json' -d '{"label":"renamed-host"}')
check "rename daemon" "$code" "204"
check "rename persisted" "$(status "$BASE/api/daemons" >/dev/null; jq -r '.[0].label' "$BODY")" "renamed-host"

# logout revokes the session
SAVED_COOKIE=$(grep -o 'session[[:space:]].*' "$JAR" | awk '{print $NF}')
code=$(status -X POST "$BASE/api/logout")
check "logout" "$code" "204"
check "session revoked after logout (old cookie 401)" \
  "$(curl -s -o /dev/null -w '%{http_code}' --cookie "session=$SAVED_COOKIE" "$BASE/api/me")" "401"

say "Smoke test complete"

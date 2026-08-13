#!/usr/bin/env sh
# Launch Rostrum's canonical fictional fixture as a local, read-only judge
# surface. Nothing is written to the repository: the binary, JSON workspace,
# audit ledger, and logs all live under one disposable directory.
set -eu

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
ROOT=$(CDPATH='' cd -- "$SCRIPT_DIR/../.." && pwd)
PORT=${JUDGE_DEMO_PORT:-8080}
BASE="http://127.0.0.1:$PORT"
DEMO_DIR=$(mktemp -d "${TMPDIR:-/tmp}/rostrum-judge-demo.XXXXXX")
BIN="$DEMO_DIR/rostrum"
LOG="$DEMO_DIR/server.log"
SERVER_PID=""

sha256_file() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{print $1}'
	else
		echo "judge-demo: install sha256sum or shasum." >&2
		return 1
	fi
}

cleanup() {
	if [ -n "$SERVER_PID" ] && kill -0 "$SERVER_PID" 2>/dev/null; then
		kill "$SERVER_PID" 2>/dev/null || true
		wait "$SERVER_PID" 2>/dev/null || true
	fi
	rm -rf -- "$DEMO_DIR"
}

trap cleanup EXIT
trap 'exit 130' HUP INT TERM

case "$PORT" in
	''|*[!0-9]*)
		echo "judge-demo: JUDGE_DEMO_PORT must be a number." >&2
		exit 2
		;;
esac
if [ "$PORT" -lt 1024 ] || [ "$PORT" -gt 65535 ]; then
	echo "judge-demo: JUDGE_DEMO_PORT must be between 1024 and 65535." >&2
	exit 2
fi

if curl -fsS --max-time 1 "$BASE/api/health" >/dev/null 2>&1; then
	echo "judge-demo: $BASE is already in use; choose another JUDGE_DEMO_PORT." >&2
	exit 2
fi

if git -C "$ROOT" rev-parse --short=12 HEAD >/dev/null 2>&1; then
	REVISION=$(git -C "$ROOT" rev-parse --short=12 HEAD)
else
	REVISION="worktree"
fi
VERSION="judge-demo-$REVISION"
TEMPLATE="$DEMO_DIR/initial-workspace.json"
CHECKSUM="$DEMO_DIR/initial-workspace.sha256"
UPLOAD_CHECKSUMS="$DEMO_DIR/uploads.sha256"
UPLOADS="$DEMO_DIR/uploads"
ROUTING_POLICY="$SCRIPT_DIR/rules/cfp-routing.arb"
ROUTING_POLICY_SHA256=$(sha256_file "$ROUTING_POLICY")

echo "judge-demo: building a disposable Rostrum binary..."
(cd "$ROOT" && CGO_ENABLED=0 go build -trimpath -o "$BIN" .)
(cd "$ROOT" && go run ./examples/demo/prepare \
	-workspace "$TEMPLATE" \
	-checksum "$CHECKSUM" \
	-upload-checksums "$UPLOAD_CHECKSUMS" \
	-assets "$SCRIPT_DIR/assets/headshots" \
	-uploads "$UPLOADS")

# Explicit empty assignments override a developer's shell and .env file. The
# preview startup validator independently rejects credentials, identity state,
# mutable/in-memory storage, and any change to the pinned example template.
env \
	APP_ENV=development \
	APP_MODE=preview \
	INITIAL_WORKSPACE=fresh \
	INITIAL_WORKSPACE_PATH="$TEMPLATE" \
	INITIAL_WORKSPACE_SHA256= \
	INITIAL_WORKSPACE_SHA256_FILE="$CHECKSUM" \
	STORE_DRIVER=json \
	DATA_PATH="$DEMO_DIR/workspace.json" \
	UPLOAD_DIR="$UPLOADS" \
	CFP_ROUTING_POLICY_PATH="$ROUTING_POLICY" \
	CFP_ROUTING_POLICY_SHA256="$ROUTING_POLICY_SHA256" \
	CFP_ROUTING_POLICY_SHA256_FILE= \
	AUDIT_LOG_PATH="$DEMO_DIR/audit.log" \
	BACKUP_DIR="$DEMO_DIR/backups" \
	PORT="$PORT" \
	PUBLIC_URL="$BASE" \
	ROSTRUM_VERSION="$VERSION" \
	SESSION_SECRET=rostrum-judge-demo-local-only-0001 \
	PREVIEW_LABEL="Observer demo" \
	PREVIEW_MESSAGE="Explore the full organizer workspace with fictional data. Controls that create, move, publish, upload, or save are not shown." \
	MAIL_DRIVER=outbox \
	MAIL_FROM= \
	ORGANIZER_EMAILS= \
	RESET_SECRET= \
	TRUSTED_PROXY_CIDRS= \
	PRINCIPAL_ROLES= \
	DATABASE_URL= \
	RESEND_API_KEY= \
	SMTP_HOST= \
	SMTP_PORT= \
	SMTP_USER= \
	SMTP_PASSWORD= \
	ACCELEVENTS_API_KEY= \
	ACCELEVENTS_API_TOKEN= \
	ACCELEVENTS_EVENT_URL= \
	ACCELEVENTS_BASE_URL= \
	AIRTABLE_PAT= \
	AIRTABLE_BASE_ID= \
	AIRTABLE_API_BASE_URL= \
	AIRTABLE_SPEAKERS_TABLE= \
	AIRTABLE_SESSIONS_TABLE= \
	AUTH_GITHUB_CLIENT_ID= \
	AUTH_GITHUB_CLIENT_SECRET= \
	AUTH_GITHUB_HANDLES= \
	AUTH_GOOGLE_CLIENT_ID= \
	AUTH_GOOGLE_CLIENT_SECRET= \
	GOSX_STATIC_EXPORT= \
	"$BIN" >"$LOG" 2>&1 &
SERVER_PID=$!

attempt=0
until curl -fsS --max-time 2 "$BASE/api/health" >/dev/null 2>&1; do
	attempt=$((attempt + 1))
	if ! kill -0 "$SERVER_PID" 2>/dev/null; then
		echo "judge-demo: server exited during startup:" >&2
		sed -n '1,160p' "$LOG" >&2
		exit 1
	fi
	if [ "$attempt" -ge 30 ]; then
		echo "judge-demo: server did not become ready within 30 seconds." >&2
		sed -n '1,160p' "$LOG" >&2
		exit 1
	fi
	sleep 1
done

echo "judge-demo: verifying the deterministic read-only contract..."
SMOKE_EXPECTED_VERSION="$VERSION" "$SCRIPT_DIR/smoke.sh" "$BASE"

cat <<EOF

Rostrum judge demo is ready (read-only, deterministic, no credentials).

  Home                 $BASE/
  Guided product tour  $BASE/tour
  Organizer workspace  $BASE/organizer
  Public CFP            $BASE/submit/systems-forum-cfp
  Public agenda         $BASE/public/m31-systems-forum-2026/agenda
  Speaker gallery       $BASE/public/m31-systems-forum-2026/speakers
  Public calendar       $BASE/public-calendar/m31-systems-forum-2026.ics
  Public JSON           $BASE/api/v1/workspace

Every mutation is expected to return HTTP 403. Press Ctrl-C to stop; the
process and its disposable workspace will be removed automatically.
EOF

wait "$SERVER_PID"

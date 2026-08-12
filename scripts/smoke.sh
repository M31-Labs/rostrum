#!/usr/bin/env sh
# Judge-facing contract smoke test.
#
# With no argument, this script builds and boots a disposable APP_MODE=demo
# process that uses the canonical fixed fixture. With a URL, it verifies that
# exact remote deployment. Remote mode requires an immutable version match;
# set SMOKE_EXPECTED_VERSION explicitly for a release tag, otherwise the
# current Git HEAD is expected. The test never follows redirects.
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
BASE=${1:-}
SERVER_PID=""
SMOKE_DIR=$(mktemp -d "${TMPDIR:-/tmp}/rostrum-smoke.XXXXXX")
BIN="$SMOKE_DIR/rostrum"
LOG="$SMOKE_DIR/server.log"
HEADERS="$SMOKE_DIR/headers"
BODY="$SMOKE_DIR/body"
FAIL=0
IDENTITY_OK=1
LAST_CODE=""

cleanup() {
	if [ -n "$SERVER_PID" ] && kill -0 "$SERVER_PID" 2>/dev/null; then
		kill "$SERVER_PID" 2>/dev/null || true
		wait "$SERVER_PID" 2>/dev/null || true
	fi
	rm -rf -- "$SMOKE_DIR"
}

trap cleanup EXIT
trap 'exit 130' HUP INT TERM

pass() {
	printf 'smoke: OK   %s\n' "$1"
}

fail() {
	printf 'smoke: FAIL %s\n' "$1" >&2
	FAIL=1
}

identity_fail() {
	fail "$1"
	IDENTITY_OK=0
}

response_header() {
	awk -v wanted="$1" '
		{
			separator = index($0, ":")
			if (separator == 0) next
			name = substr($0, 1, separator - 1)
			if (tolower(name) != tolower(wanted)) next
			value = substr($0, separator + 1)
			sub(/^[ \t]*/, "", value)
			sub(/\r$/, "", value)
			last = value
		}
		END { if (last != "") print last }
	' "$HEADERS"
}

fetch() {
	method=$1
	path=$2
	if ! LAST_CODE=$(curl -sS --max-time 20 \
		-X "$method" \
		-D "$HEADERS" \
		-o "$BODY" \
		-w '%{http_code}' \
		"$BASE$path"); then
		fail "$method $path could not be fetched"
		LAST_CODE="000"
		return 1
	fi
	return 0
}

assert_robots() {
	label=$1
	robots=$(response_header "X-Robots-Tag")
	if [ "$robots" = "noindex, nofollow, noarchive" ]; then
		pass "$label carries X-Robots-Tag"
	else
		fail "$label X-Robots-Tag is '${robots:-missing}', want 'noindex, nofollow, noarchive'"
	fi
}

assert_page() {
	path=$1
	label=$2
	marker=$3
	if ! fetch GET "$path"; then
		return
	fi
	if [ "$LAST_CODE" != "200" ]; then
		fail "$label returned HTTP $LAST_CODE, want 200"
		return
	fi
	if grep -Fq "$marker" "$BODY"; then
		pass "$label renders '$marker'"
	else
		fail "$label is missing '$marker'"
	fi
	assert_robots "$label"
}

html_text() {
	sed 's/<[^>]*>/ /g' "$BODY" | tr '\n\r\t' '   ' | sed 's/  */ /g'
}

assert_body_text() {
	label=$1
	wanted=$2
	if html_text | grep -Fq "$wanted"; then
		pass "$label contains '$wanted'"
	else
		fail "$label is missing '$wanted'"
	fi
}

count_occurrences() {
	awk -v needle="$1" '
		{
			line = $0
			while ((position = index(line, needle)) > 0) {
				count++
				line = substr(line, position + length(needle))
			}
		}
		END { print count + 0 }
	' "$BODY"
}

extract_href() {
	prefix=$1
	awk -v prefix="$prefix" '
		{
			line = $0
			needle = "href=\"" prefix
			position = index(line, needle)
			if (position == 0) next
			value = substr(line, position + length("href=\""))
			ending = index(value, "\"")
			if (ending == 0) next
			print substr(value, 1, ending - 1)
			exit
		}
	' "$BODY"
}

assert_forbidden() {
	method=$1
	path=$2
	label=$3
	if ! fetch "$method" "$path"; then
		return
	fi
	if [ "$LAST_CODE" != "403" ]; then
		fail "$label returned HTTP $LAST_CODE, want 403"
		return
	fi
	if ! grep -Fq "read-only demo" "$BODY"; then
		fail "$label returned 403 without the read-only demo reason"
		return
	fi
	pass "$label is blocked with HTTP 403"
	assert_robots "$label"
}

if [ -z "$BASE" ]; then
	PORT=${SMOKE_PORT:-8791}
	case "$PORT" in
	''|*[!0-9]*)
		echo "smoke: SMOKE_PORT must be a number." >&2
		exit 2
		;;
	esac
	BASE="http://127.0.0.1:$PORT"
	if curl -fsS --max-time 1 "$BASE/api/health" >/dev/null 2>&1; then
		echo "smoke: $BASE is already in use; choose another SMOKE_PORT." >&2
		exit 2
	fi
	if git -C "$ROOT" rev-parse --short=12 HEAD >/dev/null 2>&1; then
		REVISION=$(git -C "$ROOT" rev-parse --short=12 HEAD)
	else
		REVISION="worktree"
	fi
	EXPECTED_VERSION="smoke-$REVISION"
	echo "smoke: building disposable demo binary..."
	(cd "$ROOT" && CGO_ENABLED=0 go build -trimpath -o "$BIN" .)
	env \
		APP_ENV=development \
		APP_MODE=demo \
		SEED=demo \
		DEMO_MODE=true \
		STORE_DRIVER=json \
		DATA_PATH="$SMOKE_DIR/workspace.json" \
		AUDIT_LOG_PATH="$SMOKE_DIR/audit.log" \
		BACKUP_DIR="$SMOKE_DIR/backups" \
		PORT="$PORT" \
		PUBLIC_URL="$BASE" \
		ROSTRUM_VERSION="$EXPECTED_VERSION" \
		SESSION_SECRET=rostrum-smoke-local-only-session-0001 \
		MAIL_DRIVER=outbox \
		MAIL_FROM= \
		ORGANIZER_EMAILS= \
		RESET_SECRET= \
		DATABASE_URL= \
		RESEND_API_KEY= \
		SMTP_HOST= \
		SMTP_PORT= \
		SMTP_USER= \
		SMTP_PASSWORD= \
		ACCELEVENTS_API_KEY= \
		ACCELEVENTS_API_TOKEN= \
		ACCELEVENTS_EVENT_URL= \
		AIRTABLE_PAT= \
		AIRTABLE_BASE_ID= \
		AUTH_GITHUB_CLIENT_ID= \
		AUTH_GITHUB_CLIENT_SECRET= \
		AUTH_GOOGLE_CLIENT_ID= \
		AUTH_GOOGLE_CLIENT_SECRET= \
		GOSX_STATIC_EXPORT= \
		"$BIN" >"$LOG" 2>&1 &
	SERVER_PID=$!
	attempt=0
	until curl -fsS --max-time 2 "$BASE/api/health" >/dev/null 2>&1; do
		attempt=$((attempt + 1))
		if ! kill -0 "$SERVER_PID" 2>/dev/null; then
			echo "smoke: demo server exited during startup:" >&2
			sed -n '1,160p' "$LOG" >&2
			exit 1
		fi
		if [ "$attempt" -ge 30 ]; then
			echo "smoke: demo server did not become ready within 30 seconds." >&2
			sed -n '1,160p' "$LOG" >&2
			exit 1
		fi
		sleep 1
	done
else
	BASE=${BASE%/}
	EXPECTED_VERSION=${SMOKE_EXPECTED_VERSION:-}
	if [ -z "$EXPECTED_VERSION" ]; then
		if git -C "$ROOT" rev-parse HEAD >/dev/null 2>&1; then
			EXPECTED_VERSION=$(git -C "$ROOT" rev-parse HEAD)
		else
			echo "smoke: remote mode requires SMOKE_EXPECTED_VERSION outside a Git worktree." >&2
			exit 2
		fi
	fi
fi

echo "smoke: checking $BASE (expected version $EXPECTED_VERSION)"

# Establish deployment identity before attempting any mutation probe. This is
# what makes remote smoke fail loudly against an old framework build or a live
# production instance without risking a write to it.
if fetch GET "/api/health"; then
	if [ "$LAST_CODE" != "200" ]; then
		identity_fail "/api/health returned HTTP $LAST_CODE, want 200"
	else
		ACTUAL_VERSION=$(sed -n 's/.*"version":"\([^"]*\)".*/\1/p' "$BODY")
		if grep -Fq '"app":"Rostrum"' "$BODY" && grep -Fq '"ok":true' "$BODY"; then
			pass "/api/health identifies Rostrum"
		else
			identity_fail "/api/health does not identify a healthy Rostrum app"
		fi
		if [ "$ACTUAL_VERSION" = "$EXPECTED_VERSION" ]; then
			pass "/api/health version is $ACTUAL_VERSION"
		else
			identity_fail "/api/health version is '${ACTUAL_VERSION:-missing}', want '$EXPECTED_VERSION' (wrong or stale deployment)"
		fi
		ROBOTS=$(response_header "X-Robots-Tag")
		if [ "$ROBOTS" = "noindex, nofollow, noarchive" ]; then
			pass "/api/health carries the read-only demo robots policy"
		else
			identity_fail "/api/health X-Robots-Tag is '${ROBOTS:-missing}', so this is not the expected demo deployment"
		fi
	fi
else
	IDENTITY_OK=0
fi

if fetch GET "/api/v1/workspace"; then
	if [ "$LAST_CODE" != "200" ]; then
		identity_fail "/api/v1/workspace returned HTTP $LAST_CODE, want 200"
	elif grep -Fq '"publishedSessions":6' "$BODY" && grep -Fq '"publishedSpeakers":6' "$BODY"; then
		pass "/api/v1/workspace has canonical seeded public counts (6 sessions, 6 speakers)"
	else
		identity_fail "/api/v1/workspace does not contain canonical seeded counts (wrong fixture or stale deployment)"
	fi
else
	IDENTITY_OK=0
fi

assert_page "/" "public home" "The calm way to run a complicated program."
if grep -Fq '<html lang="en"' "$BODY"; then
	pass "file-router document declares English"
else
	fail "file-router document is missing lang=en"
fi
assert_body_text "public home seed" "12 proposals"
assert_body_text "public home seed" "10 participants"
assert_body_text "public home seed" "8 sessions"

assert_page "/organizer" "organizer overview" "Agenda pulse"
if ! grep -Fq "Read-only demo" "$BODY"; then
	identity_fail "organizer overview is missing the Read-only demo banner"
else
	pass "organizer overview exposes the Read-only demo banner"
fi

assert_page "/organizer/forms" "organizer forms" "Question structure"
assert_page "/organizer/submissions" "organizer submissions" "Proposal inventory"
assert_body_text "organizer submissions seed" "12 shown of 12 proposals"
assert_page "/organizer/review" "organizer review" "Multi-round review"
assert_page "/organizer/speakers" "organizer speakers" "Participant operations"
assert_body_text "organizer speakers seed" "10 shown of 10"
assert_page "/organizer/agenda" "organizer agenda" "Conflict-aware scheduling"
assert_body_text "organizer agenda seed" "8 sessions"
assert_page "/organizer/communications" "organizer communications" "Reusable templates"
assert_page "/organizer/portal" "organizer portal operations" "Live completion matrix"
assert_page "/organizer/embeds" "organizer embeds" "Publish everywhere"
assert_page "/organizer/integrations" "organizer integrations" "One-way publishing"
assert_page "/organizer/settings" "organizer settings" "Event configuration"

assert_page "/login" "identity explanation" "This hosted preview uses fictional data and is read-only."
assert_page "/submit/systems-forum-cfp" "submitter CFP" "Submit proposal"

# The product tour is also the safe credential-free bridge into the two
# signed-link personas. Extract its generated URLs rather than hardcoding a
# token or teaching the smoke test the signing algorithm.
assert_page "/tour" "product tour" "Follow the program all the way through."
REVIEWER_PATH=$(extract_href "/review/")
SPEAKER_PATH=$(extract_href "/portal/")
if [ -n "$REVIEWER_PATH" ]; then
	assert_page "$REVIEWER_PATH" "signed reviewer desk" "Weighted criteria"
else
	fail "product tour does not expose a signed reviewer route in demo mode"
fi
if [ -n "$SPEAKER_PATH" ]; then
	assert_page "$SPEAKER_PATH" "signed speaker portal" "Your checklist"
else
	fail "product tour does not expose a signed speaker route in demo mode"
fi

# Invalid links must stay non-enumerating even though the tour's valid,
# fixture-only links above make both personas inspectable.
assert_page "/portal/spk_maya" "speaker portal access boundary" "Check your email for your portal link"
assert_page "/review/not-a-valid-token" "reviewer access boundary" "Check your email for your review link"

assert_page "/public/m31-systems-forum-2026/agenda" "public agenda" "Build your day."
assert_body_text "public agenda seed" "6 sessions across 4 tracks"
assert_page "/public/m31-systems-forum-2026/speakers" "public speaker gallery" "builders sharing work from the field."
assert_body_text "public speaker gallery seed" "6 builders sharing work from the field."
if grep -Fq 'src="/demo-headshots/spk_maya.webp"' "$BODY" && ! grep -Fq 'example.com/' "$BODY"; then
	pass "public speaker gallery renders an approved portrait without fictional dead-end links"
else
	fail "public speaker gallery is missing an approved portrait or still contains a fictional outbound link"
fi
assert_page "/public/m31-systems-forum-2026/agenda?embed=1" "agenda embed" "Build your day."
assert_page "/public/m31-systems-forum-2026/speakers?embed=1" "speaker embed" "builders sharing work from the field."

if fetch GET "/demo-headshots/spk_maya.webp"; then
	HEADSHOT_TYPE=$(response_header "Content-Type")
	if [ "$LAST_CODE" = "200" ] && [ "$HEADSHOT_TYPE" = "image/webp" ]; then
		pass "approved fictional portrait asset is deployable"
	else
		fail "approved fictional portrait returned HTTP $LAST_CODE and type '${HEADSHOT_TYPE:-missing}', want 200 image/webp"
	fi
	assert_robots "approved fictional portrait"
fi

if fetch GET "/public-calendar/m31-systems-forum-2026.ics"; then
	CALENDAR_TYPE=$(response_header "Content-Type")
	EVENT_COUNT=$(count_occurrences "BEGIN:VEVENT")
	if [ "$LAST_CODE" = "200" ] && [ "$CALENDAR_TYPE" = "text/calendar; charset=utf-8" ] && \
		[ "$EVENT_COUNT" = "6" ] && grep -Fq "BEGIN:VCALENDAR" "$BODY"; then
		pass "public calendar exposes exactly 6 published VEVENT records"
	else
		fail "public calendar returned HTTP $LAST_CODE, type '${CALENDAR_TYPE:-missing}', and $EVENT_COUNT events; want 200 text/calendar with 6"
	fi
	assert_robots "public calendar"
fi

if fetch GET "/api/v1/schedule"; then
	if [ "$LAST_CODE" != "200" ]; then
		fail "/api/v1/schedule returned HTTP $LAST_CODE, want 200"
	else
		PUBLISHED_COUNT=$(count_occurrences '"status":"published"')
		if [ "$PUBLISHED_COUNT" = "6" ] && ! grep -Fq '"status":"draft"' "$BODY"; then
			pass "/api/v1/schedule exposes exactly 6 published sessions and no drafts"
		else
			fail "/api/v1/schedule exposes $PUBLISHED_COUNT published sessions or includes drafts; want exactly 6 published only"
		fi
	fi
fi

if fetch GET "/api/v1/speakers"; then
	if [ "$LAST_CODE" != "200" ]; then
		fail "/api/v1/speakers returned HTTP $LAST_CODE, want 200"
	else
		SPEAKER_COUNT=$(count_occurrences '"sessionIds":')
		if [ "$SPEAKER_COUNT" = "6" ] && ! grep -Fq '"email"' "$BODY"; then
			pass "/api/v1/speakers exposes exactly 6 public speakers and no email field"
		else
			fail "/api/v1/speakers exposes $SPEAKER_COUNT speakers or an email field; want 6 public-only speakers"
		fi
	fi
fi

# POST only after immutable version, robots policy, canonical fixture counts,
# and the in-product read-only banner have all identified this as the isolated
# demo. A stale or live remote is reported above and never receives a write.
if [ "$IDENTITY_OK" -eq 1 ]; then
	assert_forbidden GET "/auth/magic-link" "identity mutation surface"
	assert_forbidden GET "/organizer/export/submissions.csv" "private export surface"

	# A mutation method aimed at a deliberately nonexistent path cannot change
	# live application state. In demo mode the global gate still catches it
	# before routing, so this is a safe final proof before sending representative
	# POSTs to real action paths.
	DEMO_GATE_OK=0
	if fetch DELETE "/__rostrum_smoke_read_only_probe__"; then
		if [ "$LAST_CODE" = "403" ] && grep -Fq "read-only demo" "$BODY"; then
			pass "global read-only mutation gate blocks a non-routable DELETE"
			assert_robots "global read-only mutation gate"
			DEMO_GATE_OK=1
		else
			fail "global read-only mutation gate returned HTTP $LAST_CODE without the demo reason"
		fi
	fi
	if [ "$DEMO_GATE_OK" -eq 1 ]; then
		assert_forbidden POST "/submit/systems-forum-cfp/__actions/submitProposal" "public CFP mutation"
		assert_forbidden POST "/demo/reset" "demo reset mutation"
	else
		fail "real action POST probes skipped because the global demo mutation gate was not proven"
	fi
else
	fail "mutation probes skipped because the target did not prove it is the expected isolated demo"
fi

if [ "$FAIL" -ne 0 ]; then
	echo "smoke: FAILED — target is incomplete, stale, or not the deterministic read-only judge demo." >&2
	exit 1
fi

echo "smoke: PASS — deterministic fixture, advertised routes, public projections, and read-only boundary verified."

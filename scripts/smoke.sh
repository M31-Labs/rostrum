#!/usr/bin/env sh
# Rendered-HTML smoke test: boot the built server and assert the headline
# flows render real input controls. A form that compiles but renders zero
# inputs has shipped before and passed a route-status audit; this asserts the
# rendered result, not just a 200. Run it in the deploy pipeline after build.
#
# Usage: scripts/smoke.sh [base-url]
#   With no argument it builds a temporary binary, runs it on a spare port in
#   DEMO_MODE=memory, and tests that. With a URL it tests a running instance
#   (for example the live site after a deploy).
set -eu

BASE="${1:-}"
SRV_PID=""
TMPBIN=""
LOGFILE=""
EXPECTED_VERSION=""
COOKIEJAR="$(mktemp)"

cleanup() {
	[ -n "$SRV_PID" ] && kill "$SRV_PID" 2>/dev/null || true
	[ -n "$TMPBIN" ] && rm -f "$TMPBIN" || true
	[ -n "$LOGFILE" ] && rm -f "$LOGFILE" || true
	rm -f "$COOKIEJAR"
}
trap cleanup EXIT INT TERM

if [ -z "$BASE" ]; then
	TMPBIN="$(mktemp)"
	LOGFILE="$(mktemp)"
	echo "smoke: building static binary..."
	CGO_ENABLED=0 go build -o "$TMPBIN" .
	PORT=8791
	BASE="http://localhost:$PORT"
	echo "smoke: starting server on $BASE..."
	# ORGANIZER_EMAILS is explicitly empty so the identity plane's break-glass
	# bootstrap arms and logs a one-time /setup URL to $LOGFILE; the block
	# below uses it to sign in as the first organizer.
	EXPECTED_VERSION="smoke"
	PORT="$PORT" DEMO_MODE=memory PUBLIC_URL="$BASE" ROSTRUM_VERSION="$EXPECTED_VERSION" ORGANIZER_EMAILS="" "$TMPBIN" >"$LOGFILE" 2>&1 &
	SRV_PID=$!
	# Wait for readiness.
	i=0
	until curl -sf --max-time 2 "$BASE/api/health" >/dev/null 2>&1; do
		i=$((i + 1))
		[ "$i" -gt 30 ] && { echo "smoke: server did not become ready"; exit 1; }
		sleep 0.3
	done
fi

fail=0

# A release must be identifiable by its own immutable version, not merely the
# GoSX framework it happened to compile with. Local mode pins an expected
# value; a remote smoke target can be checked against its deployment record.
if [ -n "$EXPECTED_VERSION" ]; then
	health="$(curl -s --max-time 20 "$BASE/api/health")"
	if printf '%s' "$health" | grep -q "\"version\":\"$EXPECTED_VERSION\""; then
		echo "smoke: OK   /api/health (version=$EXPECTED_VERSION)"
	else
		echo "smoke: FAIL /api/health (expected version=$EXPECTED_VERSION)"
		fail=1
	fi
fi

# check <route> <min-inputs> <min-selects> <min-textareas> <required-name...>
# Sends any cookies COOKIEJAR already holds, so an authenticated route (see
# the break-glass block below) renders its real, signed-in content.
check() {
	route="$1"; min_in="$2"; min_sel="$3"; min_ta="$4"; shift 4
	html="$(curl -s --max-time 20 -b "$COOKIEJAR" -c "$COOKIEJAR" "$BASE$route")"
	# Count occurrences, not matching lines (minified HTML packs controls per line).
	nin="$(printf '%s' "$html" | grep -o '<input' | wc -l | tr -d ' ')"
	nsel="$(printf '%s' "$html" | grep -o '<select' | wc -l | tr -d ' ')"
	nta="$(printf '%s' "$html" | grep -o '<textarea' | wc -l | tr -d ' ')"
	ok=1
	[ "$nin" -ge "$min_in" ] || ok=0
	[ "$nsel" -ge "$min_sel" ] || ok=0
	[ "$nta" -ge "$min_ta" ] || ok=0
	for name in "$@"; do
		printf '%s' "$html" | grep -q "name=\"$name\"" || { ok=0; echo "smoke: $route MISSING field name=\"$name\""; }
	done
	if [ "$ok" -eq 1 ]; then
		echo "smoke: OK   $route (inputs=$nin selects=$nsel textareas=$nta)"
	else
		echo "smoke: FAIL $route (inputs=$nin/$min_in selects=$nsel/$min_sel textareas=$nta/$min_ta)"
		fail=1
	fi
}

# Break-glass sign-in: only possible against a server we started ourselves,
# because it needs the /setup URL the process logs once at startup
# (identity-plane spec AU-5). A remote target (an already-deployed instance)
# skips this — it either already has ORGANIZER_EMAILS configured or the
# token from its own startup is long gone — and so skips the organizer
# check below rather than fail for lack of credentials this script cannot
# have.
if [ -n "$LOGFILE" ]; then
	echo "smoke: bootstrapping an organizer session via break-glass setup..."
	setup_url="$(grep -o 'http[^ ]*/setup?token=[A-Za-z0-9_-]*' "$LOGFILE" | tail -1)"
	if [ -z "$setup_url" ]; then
		echo "smoke: FAIL no break-glass /setup URL was logged at startup"
		fail=1
	else
		setup_token="$(printf '%s' "$setup_url" | sed -E 's/.*token=//')"
		setup_html="$(curl -s --max-time 20 -b "$COOKIEJAR" -c "$COOKIEJAR" "$setup_url")"
		csrf_token="$(printf '%s' "$setup_html" | grep -o 'name="csrf-token" content="[^"]*"' | head -1 | sed -E 's/.*content="([^"]*)".*/\1/')"
		if [ -z "$csrf_token" ]; then
			echo "smoke: FAIL /setup did not render a CSRF token"
			fail=1
		else
			curl -s -o /dev/null --max-time 20 -b "$COOKIEJAR" -c "$COOKIEJAR" \
				-X POST "$BASE/setup/__actions/completeSetup" \
				--data-urlencode "csrf_token=$csrf_token" \
				--data-urlencode "token=$setup_token" \
				--data-urlencode "email=smoke@example.com" \
				--data-urlencode "name=Smoke Test"
			echo "smoke: OK   break-glass setup completed"
		fi
	fi
fi

# The CFP submit form must render its schema fields, not just the hidden pair.
check "/submit/systems-forum-cfp" 6 2 2 title abstract email first_name category
# /login must always offer the magic-link form and the passkey button.
check "/login" 3 0 0 email csrf_token
# The organizer review surface must render its scoring controls. Reachable
# only with the organizer session COOKIEJAR now holds (identity-plane
# spec AU-5); an anonymous request redirects to /login instead.
if [ -n "$LOGFILE" ]; then
	check "/organizer/review" 1 0 0
else
	echo "smoke: SKIP /organizer/review (remote target: no way to bootstrap an organizer session)"
fi
# The speaker portal needs a signed link, so without a key it correctly renders
# the "check your email" page. Assert only that the route serves a page; the
# render-nothing bug class is caught by the CFP and review control checks.
pcode="$(curl -s -o /dev/null -w '%{http_code}' --max-time 20 "$BASE/portal/spk_maya")"
if [ "$pcode" = "200" ]; then
	echo "smoke: OK   /portal/spk_maya (route serves 200)"
else
	echo "smoke: FAIL /portal/spk_maya (HTTP $pcode)"
	fail=1
fi

if [ "$fail" -ne 0 ]; then
	echo "smoke: FAILED — a headline flow renders no usable controls; do not deploy."
	exit 1
fi
echo "smoke: all headline flows render real controls."

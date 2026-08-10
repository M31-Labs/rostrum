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

cleanup() {
	[ -n "$SRV_PID" ] && kill "$SRV_PID" 2>/dev/null || true
	[ -n "$TMPBIN" ] && rm -f "$TMPBIN" || true
}
trap cleanup EXIT INT TERM

if [ -z "$BASE" ]; then
	TMPBIN="$(mktemp)"
	echo "smoke: building static binary..."
	CGO_ENABLED=0 go build -o "$TMPBIN" .
	PORT=8791
	BASE="http://localhost:$PORT"
	echo "smoke: starting server on $BASE..."
	PORT="$PORT" DEMO_MODE=memory PUBLIC_URL="$BASE" "$TMPBIN" >/dev/null 2>&1 &
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

# check <route> <min-inputs> <min-selects> <min-textareas> <required-name...>
check() {
	route="$1"; min_in="$2"; min_sel="$3"; min_ta="$4"; shift 4
	html="$(curl -s --max-time 20 "$BASE$route")"
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

# The CFP submit form must render its schema fields, not just the hidden pair.
check "/submit/systems-forum-cfp" 6 2 2 title abstract email first_name category
# The organizer review surface must render its scoring controls.
check "/organizer/review" 1 0 0
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

---
description: A verified local fast path, five-minute demo script, persona walkthroughs, and evidence matrix for Rostrum.
nav_order: "02 / 08"
eyebrow: Evaluate the complete product
---

# Judge and organizer guide

<!-- markdownlint-disable MD013 -->

This guide is the shortest reliable route through Rostrum. It names what a
judge can verify in the hosted preview, what requires a local run, and where
the repository evidence lives.

## Choose the right path

| Path | Use it for | Do not expect |
| --- | --- | --- |
| Local judge demo | Deterministic, smoke-verified visual and persona inspection | Mutations; it deliberately reproduces the hosted read-only posture |
| Fresh interactive run | Organizer bootstrap and mutation testing from a clean starter CFP | Fictional proposals, people, reviews, or schedule records |
| Hosted preview | Visual inspection of a known fictional workspace after preflight; the CFP also has a client-only submit walkthrough | Sign-in, persisted submission, upload, import/export, reset, or any mutation |

### Local judge demo (recommended first)

Requirements: Go 1.26 or newer, Make, `curl`, and a POSIX shell (macOS,
Linux, or WSL on Windows).

```bash
git clone https://github.com/M31-Labs/rostrum.git
cd rostrum
make judge-demo
```

The helper builds a disposable binary and fixture outside the repository, runs
the same deterministic read-only contract used for the hosted preview, and
prints the canonical routes. Open `/tour` first. Its reviewer and speaker stops
carry signed persona links generated for that disposable, independently
read-only fixture.

### Fresh interactive run

Requirements: Go 1.26 or newer and Make. No database, Node toolchain, or
provider credential is required.

```bash
git clone https://github.com/M31-Labs/rostrum.git
cd rostrum
APP_MODE=live make dev
```

The process prints:

```text
Rostrum has no organizer yet. Finish setup at: http://localhost:8080/setup?token=...
Rostrum listening on http://localhost:8080 (data: :memory:)
```

Open the complete setup URL from the terminal. Enter a name and email address.
Rostrum creates the first `organizer` principal, consumes the token, and signs
that browser in. The token is single-use. This command deliberately uses
in-memory storage, so stopping it discards the workspace and a later clean run
can bootstrap again. A durable self-host does not re-arm setup after its first
organizer is stored.

Useful routes:

- Product tour: <http://localhost:8080/tour>
- Workspace: <http://localhost:8080/organizer>
- Starter CFP: <http://localhost:8080/submit/call-for-proposals>
- Agenda board: <http://localhost:8080/organizer/agenda>
- Public agenda: <http://localhost:8080/public/your-event/agenda>
- Speakers: <http://localhost:8080/public/your-event/speakers>
- Public calendar: <http://localhost:8080/public-calendar/your-event.ics>
- API directory: <http://localhost:8080/api/v1/workspace>

The starter has no fictional activity. Create controlled records through the
organizer and public flows, or follow the [self-hosting manual](self-hosting.md)
when you want those changes to persist.

### Hosted preview preflight

The convenience preview is <https://rostrum.m31labs.dev>. Verify it before the
demo, because deployment state can lag source state:

```bash
curl -fsS https://rostrum.m31labs.dev/api/health
curl -fsS https://rostrum.m31labs.dev/api/v1/workspace
curl -sSI https://rostrum.m31labs.dev/organizer
```

A judge-ready response has all of these properties:

- Health returns `"ok": true` and an immutable Rostrum release identifier,
  not `dev` or a framework version used as the app version.
- Workspace returns slug `m31-systems-forum-2026` with non-zero
  `publishedSessions` and `publishedSpeakers`.
- `/organizer` returns 200 without redirecting to `/login`.
- Responses include `X-Robots-Tag: noindex, nofollow, noarchive`.
- The tour, CFP, organizer agenda, public agenda, speaker gallery, and public
  calendar return 200.

If any check fails, use the local path. A broken or stale deployment is not
evidence against the repository and must not be presented as the current
build.

## Five-minute demo script

This sequence uses the fictional read-only example launched by
`make judge-demo` and keeps the narrative focused.

### 0:00–0:40 — The promise

Open `/tour`, frame the five-person handoff, then enter `/organizer`. Point to
the command center: submission pipeline, review coverage, speaker readiness,
and schedule health are one program—not four spreadsheets. Press `Ctrl/Cmd K`
to show the workspace switcher.

**Claim to make:** Rostrum carries one canonical record from call to published
agenda, and it keeps governed decisions explainable.

### 0:40–1:25 — Intake that explains itself

Open **Forms & routing**, then the public CFP. Choose `Workshop` and show the
conditional logistics question. Type into the proposal fields and click **Save
draft** or **Submit proposal** to show the client-only receipt; no network
write occurs. Return to a fictional example submission and show its queue,
owner, track, fired rule, and readable trace.

**Claim to make:** form visibility and proposal routing are policy-backed and
server-validated; the interface does not hide why a proposal went somewhere.
The hosted CFP interaction is a safe visual walkthrough, not a stored intake.

### 1:25–2:10 — Human review with provenance

Open **Review**. Show the weighted rubric, rounds, roster, assignment coverage,
company-conflict exclusions, and a signed reviewer link. Point out that only
attributed human evaluations contribute to displayed coverage and aggregates.

**Claim to make:** review is multi-round, rubric-driven, assignment-aware, and
auditable. Do not claim automated or model-generated scoring.

### 2:10–3:10 — Scheduling with a publication gate

Open **Agenda**. Show the unscheduled bank, alternate views, and the fictional
speaker/room conflict explanation. The preview deliberately omits drag,
keyboard-move, and publish controls; point to the visible conflict inspector
and explain that the fresh live path exposes those controls while hard
collisions still block publication.

**Claim to make:** the agenda protects the public schedule from known hard
conflicts while preserving an organizer-readable reason.

### 3:10–4:05 — The speaker side

Open **Portal & tasks**. Show the readiness matrix, scoped tasks, pending file
approval, and the live status surface. In a full local walkthrough, submit a
new CFP proposal and follow its signed portal journey.

**Claim to make:** speakers do not manage passwords; signed links scope them to
their own profile, tasks, uploads, and calendar.

### 4:05–5:00 — Publish once, use everywhere

Open the public agenda, save a session to the device-local itinerary, download
the public whole-event calendar, and show the speaker gallery. End at
`/api/v1/workspace`, then mention embeds, private speaker calendars, the
communications outbox, and checksummed organizer exports.

**Claim to make:** the committed schedule is projected to every public output;
draft sessions and private proposal/review data stay behind the boundary.

## Persona walkthroughs

| Persona | Start here | What to verify |
| --- | --- | --- |
| Live organizer | `/setup?token=…`, then `/organizer` in the fresh live run | Full workspace, forms, status decisions, agenda, tasks, communications, settings |
| Preview observer | `/organizer` in the evaluation preview | Anonymous organizer-facing inspection without mutations or PII export |
| Chair workflow | **Review** in the example; a real chair role in live | Governed final-decision and override evidence; sensitive export remains role-gated |
| Reviewer | Signed URL copied from `/organizer/review` | Only that reviewer's assignments and attributed rubric submission |
| Speaker | Signed portal URL created by submission/invitation | Own profile, tasks, uploads, resources, and calendar |
| Attendee | `/public/{slug}/agenda` | Published schedule, speaker gallery, device-local itinerary |
| Operator | `docs/deployment.md` | Immutable version, storage, secrets, proxy, health, recovery evidence |

The anonymous hosted preview bypasses organizer sign-in only inside
`APP_MODE=preview`; a store-level read-only wrapper and route gate still reject
mutations and sensitive operations.

## Capability and evidence matrix

| Capability | Demonstration | Repository evidence | Boundary |
| --- | --- | --- | --- |
| CFP schema and conditional visibility | `/submit/systems-forum-cfp`, `/organizer/forms` | [`app/submit/page_server_test.go`](https://github.com/M31-Labs/rostrum/blob/main/app/submit/page_server_test.go), [`app/organizer/forms/page_server_test.go`](https://github.com/M31-Labs/rostrum/blob/main/app/organizer/forms/page_server_test.go), [`rules/form-visibility.arb`](https://github.com/M31-Labs/rostrum/blob/main/rules/form-visibility.arb) | Conditions are constrained `equals` → `show` rules, not arbitrary scripts |
| Governed routing | Fictional example submission detail | [`examples/demo/rules/cfp-routing.arb`](https://github.com/M31-Labs/rostrum/blob/main/examples/demo/rules/cfp-routing.arb), [`rules/engine.go`](https://github.com/M31-Labs/rostrum/blob/main/rules/engine.go) | Core defaults to unassigned program triage; the example loads its fictional category policy by an exact SHA-256 pin |
| Human review | `/organizer/review`, signed `/review/{token}` | [`app/organizer/review/management_test.go`](https://github.com/M31-Labs/rostrum/blob/main/app/organizer/review/management_test.go), [`app/review/review_server_test.go`](https://github.com/M31-Labs/rostrum/blob/main/app/review/review_server_test.go), [`rules/review-governance.arb`](https://github.com/M31-Labs/rostrum/blob/main/rules/review-governance.arb) | Human evaluations alone drive coverage/aggregates; no automated scoring claim |
| Conflict-aware agenda | `/organizer/agenda` | [`app/organizer/agenda/page_server_test.go`](https://github.com/M31-Labs/rostrum/blob/main/app/organizer/agenda/page_server_test.go), [`internal/domain/conflicts_test.go`](https://github.com/M31-Labs/rostrum/blob/main/internal/domain/conflicts_test.go), [`rules/schedule-conflicts.arb`](https://github.com/M31-Labs/rostrum/blob/main/rules/schedule-conflicts.arb) | Hard speaker/room conflicts block publish; warnings inform |
| Speaker tasks and uploads | `/organizer/portal`, signed `/portal/{speaker}` | [`app/organizer/portal/page_server_test.go`](https://github.com/M31-Labs/rostrum/blob/main/app/organizer/portal/page_server_test.go), [`app/portal/page_server_test.go`](https://github.com/M31-Labs/rostrum/blob/main/app/portal/page_server_test.go), [`internal/archive/approved_uploads_test.go`](https://github.com/M31-Labs/rostrum/blob/main/internal/archive/approved_uploads_test.go) | 10 MiB app limit, allow-listed file types, private storage and downloads |
| Durable communications | `/organizer/communications` | [`internal/communications/runner_test.go`](https://github.com/M31-Labs/rostrum/blob/main/internal/communications/runner_test.go), [`internal/mail/`](https://github.com/M31-Labs/rostrum/tree/main/internal/mail/) | Outbox is local evidence; real Resend/SMTP delivery needs operator credentials |
| External publishing | `/organizer/integrations` dry runs and ledger | [`internal/accelevents/client_test.go`](https://github.com/M31-Labs/rostrum/blob/main/internal/accelevents/client_test.go), [`internal/airtable/airtable_test.go`](https://github.com/M31-Labs/rostrum/blob/main/internal/airtable/airtable_test.go) | One-way only; live provider acceptance needs restricted operator credentials |
| Public publishing | Public agenda/gallery and `/api/v1/*` | [`internal/publicapi/public_test.go`](https://github.com/M31-Labs/rostrum/blob/main/internal/publicapi/public_test.go), [`internal/present/public_test.go`](https://github.com/M31-Labs/rostrum/blob/main/internal/present/public_test.go) | Only published sessions and attached speakers are serialized |
| Calendar output | Public event calendar, signed speaker calendar, and communication attachment | [`internal/calendar/ics_test.go`](https://github.com/M31-Labs/rostrum/blob/main/internal/calendar/ics_test.go), [`internal/mail/invite_test.go`](https://github.com/M31-Labs/rostrum/blob/main/internal/mail/invite_test.go) | Both include published sessions only; speaker feed remains private |
| Storage portability | Select `json`, `sqlite`, or `postgres` | [`internal/store/json_test.go`](https://github.com/M31-Labs/rostrum/blob/main/internal/store/json_test.go), [`internal/store/sql_test.go`](https://github.com/M31-Labs/rostrum/blob/main/internal/store/sql_test.go) | Exact external database endpoint still needs acceptance testing |
| Export and recovery | Settings export/import/archive | [`internal/archive/workspace_test.go`](https://github.com/M31-Labs/rostrum/blob/main/internal/archive/workspace_test.go), [`main_test.go`](https://github.com/M31-Labs/rostrum/blob/main/main_test.go) | Full upload recovery is a stopped-process procedure |
| Independent audit ledger | Mutate, then inspect configured ledger | [`internal/audit/log_test.go`](https://github.com/M31-Labs/rostrum/blob/main/internal/audit/log_test.go), [`internal/store/audit_test.go`](https://github.com/M31-Labs/rostrum/blob/main/internal/store/audit_test.go) | State commit precedes ledger append; not a cross-store transaction |
| Read-only hosted posture | Correctly configured preview | [`internal/previewmode/config_test.go`](https://github.com/M31-Labs/rostrum/blob/main/internal/previewmode/config_test.go), [`internal/store/readonly_test.go`](https://github.com/M31-Labs/rostrum/blob/main/internal/store/readonly_test.go), [`preview_observer_contract_test.go`](https://github.com/M31-Labs/rostrum/blob/main/preview_observer_contract_test.go) | A deployment preflight is still required |
| Release and example gates | `make release-check` | [`cmd/sizecheck/main.go`](https://github.com/M31-Labs/rostrum/blob/main/cmd/sizecheck/main.go), [`examples/demo/smoke.sh`](https://github.com/M31-Labs/rostrum/blob/main/examples/demo/smoke.sh) | Performance budget measurement is a separate operator run |

## Verification commands

Install the pinned contributor tools, then run:

```bash
go install m31labs.dev/gosx/cmd/gosx@v0.38.1
go install m31labs.dev/arbiter/cmd/arbiter@v1.9.0

make check
make smoke
make size-budget
make judge-demo
```

`make check` covers formatting, policy validation, `go vet`, unit tests, and
race tests. `make smoke` prepares the fictional fixture outside core, boots a
temporary deterministic `APP_MODE=preview` process, and checks
organizer/persona/public/embed/API/calendar routes, expected counts, no-index
headers, and mutation refusal. `make size-budget` builds the production bundle
and checks committed route/runtime limits. `make judge-demo` launches that same
read-only example only after the contract passes.

## Known evaluation boundaries

- One instance operates one event workspace; there is no multi-tenant account
  plane.
- The hosted preview is read-only by design and is not the path for feature
  mutations.
- Real email, external Postgres, and live Accelevents/Airtable publishing need
  operator-owned credentials and separate acceptance evidence.
- Rate limits are process-local and reset on restart.
- The full archive restore procedure stops the process before restoring upload
  files.
- The app exposes a small documented JSON API, not an OpenAPI document.
- Generated GoSX browser runtime assets are used for focused islands; Rostrum
  ships no hand-written browser JavaScript.

For the production gate rather than the judge route, continue to
[launch readiness](launch-readiness.md).

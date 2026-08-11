# Rostrum launch-readiness specification

**Status:** the implementation sweep is complete. Credential-backed and
live-service acceptance remains operator-owned. This document is the release
gate for the private Rostrum repository as of 2026-08-10; it does not authorize
making the repository or a deployment public.

## Release decision

Rostrum can proceed to a private production-candidate deployment after the
operator checklist in this document is completed and evidenced. It is not yet
declared publicly launch-ready: sending real email, exercising the selected
database and provider configuration, and approving the deployed release require
credentials and infrastructure that are outside this repository.

Rostrum is always canonical. JSON, SQLite, and Postgres are alternative
canonical storage backends. Airtable is a one-way operational projection only;
it cannot alter review, scheduling, identity, archive, or audit state.

## Implemented launch contract

### Program flow and configuration

- A CFP submission flows through governed routing, review, acceptance, an
  unscheduled session bank, conflict-aware scheduling, publication, public
  API, calendar, and export surfaces.
- Manual sessions carry a selected format, speakers, and duration. Their custom
  duration survives scheduling and unscheduling.
- Organizers can create tracks, rooms, and categories with collision-safe IDs.
  A new category visibly uses the current policy fallback until a routing rule
  is added.
- Changing an event start date rebases scheduled calendar days. Changing an
  event timezone preserves each scheduled session's wall-clock time.
- Evidence: agenda, Settings, and domain regression tests; rendered-flow smoke
  test; policy provenance on governed actions.

### Canonical storage

- **JSON** is the default validated atomic-file store.
- **SQLite** uses WAL and a migration ledger.
- **Postgres** and **postgresql** use the same validated aggregate contract
  through DATABASE_URL.
- The store abstraction and SQLite/Postgres contract tests cover these
  backends. An operator still must test the exact external database endpoint
  chosen for production.

### Audit history

- Every governed mutation is recorded in aggregate audit history and appended,
  fsynced, hash-chained, and rotated in an independent JSON Lines ledger.
- Existing ledger corruption causes startup to fail rather than silently
  continuing with a new chain.
- The ledger is deliberately separate from mutable workspace state, so a
  workspace import or restore cannot rewrite the receiving instance's
  pre-existing operational record.

**Boundary:** the canonical state transaction completes before the independent
ledger append. If the ledger filesystem fails at that point, Rostrum returns an
error saying the workspace commit already succeeded. Treat it as an incident:
preserve the error evidence, repair the durable ledger path, and reconcile that
one successful mutation. This is not a distributed transaction across database
and filesystem, and the release must not claim that it is.

### Export, backup, and import

- Organizers and chairs can export checksummed structured workspace state or a
  full tar.gz archive containing state, uploads, and active/rotated audit
  segments.
- Structured workspace import validates export version, schema version,
  checksum, and domain invariants before replacement. It makes an exact
  pre-import backup, retains the newest ten, preserves local organizer
  principals/passkeys/pending magic links, and rebases upload references to
  the receiving host.
- Full archive restore is intentionally a stopped-process recovery procedure
  in v1. Extract to a private staging directory, restore workspace.json through
  the validated Settings flow, stop Rostrum, copy staged uploads into the
  receiving upload directory, then restart and verify files.
- Keep source audit segments with the recovery record. Do not splice them into
  the receiving live ledger, whose hash chain belongs to that instance.

### Email delivery

- A durable message idempotency key protects confirmation and lifecycle mail.
- MAIL_DRIVER=resend uses the Resend API with RESEND_API_KEY and MAIL_FROM.
- MAIL_DRIVER=smtp uses a standards-based SMTP server with SMTP_HOST,
  SMTP_PORT, SMTP_USER, SMTP_PASSWORD, and MAIL_FROM.
- With no complete real transport, mail records in the credential-free demo
  outbox. The outbox is not a production-delivery proof.
- Evidence: Resend request/idempotency tests, SMTP MIME/calendar tests, and
  acceptance resend regression tests.

### Airtable projection

- Airtable is an explicit, user-triggered, one-way upsert projection of
  accepted speakers and scheduled sessions.
- A credential-free dry run makes no network request. A live synchronization
  creates durable outbox entries first, writes audit events, batches ten
  records per request, backs off after failure, and uses stable Rostrum ID
  values to replay safely.
- Rostrum never pulls Airtable changes into canonical data and never sends
  speaker files as Airtable attachments.
- Required configuration: AIRTABLE_PAT, AIRTABLE_BASE_ID, and optionally
  AIRTABLE_SPEAKERS_TABLE and AIRTABLE_SESSIONS_TABLE.
- The target Speakers table needs Rostrum ID, Rostrum Schema, Name, Email,
  Role, Company, Biography, Website, and LinkedIn. The Sessions table needs
  Rostrum ID, Rostrum Schema, Title, Description, Starts At, Ends At, Room,
  Track, and Speaker IDs.
- Evidence: adapter, outbox, and HTTP contract tests.

### Access, public edge, and release identity

- Organizer, chair, and observer roles are application-enforced. Speaker and
  reviewer access remains signed-link scoped. Public APIs expose published
  material only.
- Production rejects weak/default session secrets, HTTP public URLs, and
  in-memory persistence.
- The exact GoSX preflight is gosx v0.38.1; Make targets reject another
  version before check or build.
- ROSTRUM_VERSION is a deployment-owned immutable tag or commit SHA exposed
  by GET /api/health. The Docker image accepts it as a build argument and
  local smoke asserts a known health version.

## Provider acceptance contracts

### Transactional email: choose Resend or SMTP

Set one real transport for the candidate deployment.

For Resend, configure MAIL_DRIVER=resend, RESEND_API_KEY, and a verified
MAIL_FROM identity. Keep RESEND_API_BASE_URL at its normal production endpoint
unless deliberately using a compatible test endpoint.

For SMTP, configure MAIL_DRIVER=smtp, SMTP_HOST, SMTP_PORT, SMTP_USER,
SMTP_PASSWORD, and MAIL_FROM. This is the standards-based OSS-friendly path;
do not use a provider API token as an SMTP password.

From a real external inbox and deployment:

1. Submit a test CFP.
2. Verify arrival from the intended domain, a working signed portal link, and
   no cross-speaker data exposure.
3. Repeat the triggering action and confirm idempotency avoids duplicate
   lifecycle delivery.
4. Record provider message ID/time, recipient, release version, and result in
   the launch record. Never store a provider token or SMTP password there.

### Airtable

Create a restricted Airtable Personal Access Token with record-write access
only to the target base. API keys are retired and must not be used. The current
provider references are [Personal Access Token guidance](https://support.airtable.com/docs/creating-personal-access-tokens),
[API-key retirement](https://support.airtable.com/docs/airtable-api-key-deprecation-notice),
and [API call limits](https://support.airtable.com/managing-api-call-limits-in-airtable).

1. Put AIRTABLE_PAT, AIRTABLE_BASE_ID, and any non-default table names in the
   deployment secret store.
2. Run Dry run Airtable and verify only expected accepted speakers and
   scheduled, non-cancelled sessions appear in the plan.
3. Run Sync Airtable now once. Verify stable Rostrum ID keys, intended fields,
   and agreement with canonical Rostrum data.
4. Run it again and verify an upsert/update rather than a duplicate. Check the
   visible ledger and the independent audit history.
5. Change one harmless Airtable test row and verify Rostrum does not pull it
   back. Airtable must remain non-authoritative.

### Storage

Choose one canonical backend and exercise that exact deployment configuration.

- **JSON:** mount a private durable volume for DATA_PATH, data/uploads,
  AUDIT_LOG_PATH, and BACKUP_DIR. Do not run more than one replica against the
  same JSON workspace.
- **SQLite:** boot twice against the same durable path and verify the
  WAL-backed state persists. Retain the database, WAL files, audit ledger,
  uploads, and import backups together in the backup policy.
- **Postgres:** use a non-production test DATABASE_URL, boot and restart,
  submit/import a fixture, and verify persisted aggregate state and audit
  history. Complete a separate managed-database backup/restore drill before
  promotion.

## Release verification runbook

Complete every item against the exact immutable candidate. Attach command
output, timestamp, release version, image digest, and operator to the release
record; never attach secrets or signed portal/reviewer tokens.

1. **Source and build.** Keep GitHub private. Run make check, make build, and
   make smoke. The check includes formatting, policy validation, vet, unit
   tests, race tests, and the exact GoSX version.
2. **Package.** Build the container with an immutable ROSTRUM_VERSION
   build argument. Start it with APP_ENV=production, an HTTPS PUBLIC_URL, a
   unique 32+ character SESSION_SECRET, and durable mounts.
3. **Health and edge.** Fetch GET /api/health and verify ok=true plus the
   expected immutable version. Verify TLS, WebSocket upgrade at /live,
   intended public cache headers, and no unpublished material in public APIs.
4. **Rendered smoke.** Run make smoke with SMOKE_URL set to the candidate
   URL. This remote mode intentionally covers public/login rendering only;
   use an actual organizer session for privileged acceptance.
5. **Core operator journey.** Create a test submission; verify its route and
   audit trace; assign/review/accept it; create or move its session; resolve
   a deliberate scheduling conflict; publish; and verify public
   agenda/API/calendar output. Create a manual session and verify its custom
   duration survives a board move.
6. **Recovery drill.** Export workspace JSON, import it into a clean staged
   instance, verify local organizer access remains intact, and verify the
   automatic pre-import backup. Produce one full archive and rehearse the
   stopped-process upload recovery on staging.
7. **Provider drill.** Complete the selected email test and, if enabled, the
   Airtable dry-run/upsert/replay test. Complete the selected SQLite or
   Postgres persistence test.
8. **Approval.** Record pass/fail, any exception with an expiry, image digest,
   health version, and named operator. Only then separately approve a public
   launch. Do not change repository visibility as part of this work.

## Delivered implementation work

The work packets below are implemented in this private release candidate.
They remain subject to the credential-backed acceptance steps above; their
presence does not authorize making the repository or deployment public.

### PT-4: organizer-created speaker tasks

Organizers can create, edit, assign, and retire portal tasks with a delivery
type (profile, confirmation/form, file, or headshot), due date, required flag,
and accepted-speakers-only policy. An optional initial bulk assignment includes
only speakers with an accepted-stage submission; direct assignment rechecks the
same policy in the state transaction.

Retirement is non-destructive. It removes the task from portals, reminders,
and new upload/submission authorization while retaining task completions for
audit, archive, and exports. The portal action and upload route both use the
same lifecycle-and-assignment predicate, so an old task URL cannot leak a task
or accept work from another speaker. File uploads are restricted to file and
headshot task types; normal task submission validates required fields and
declared select options server-side.

### PT-5: approved-upload bundle

`GET /organizer/export/approved-uploads.zip` is an organizer/chair-only,
audited download. It builds a deterministic ZIP containing a timestamp-free
`manifest.json` followed by only approved completion files that are regular
files under Rostrum's private upload directory. Every manifest entry contains
the completion, task, and speaker IDs, original filename, content type,
archive path, byte count, and SHA-256.

The bundle uses fixed ZIP metadata and stored entries, stable ordering, a
5,000-file / 512 MiB limit, rejects path escapes and symlinks, and rechecks
the file digest while streaming. An approved profile/form response with no
stored file is simply absent; a state reference to a missing or unsafe stored
file fails closed rather than producing a silently incomplete archive.

### CM-2, CM-4, and CM-6: durable communications

The persisted email outbox now owns scheduled task/session reminders,
administrator notifications, delivery leasing, idempotency, retry/backoff,
cancellation, and opt-out suppression. Startup and a periodic wakeup merely
drive durable due work; no delivery decision depends solely on an in-process
timer. Templates are merge-field validated, editable with retained revisions,
and system templates remain undeletable. Notification rules define trigger,
recipient, retry, and suppression policy and enqueue through the same outbox.

### RV-3: review-plan and reviewer editing

Chairs can create and revise plans, structured rubrics, review targets,
deadline/state, anonymity, attachments, reminders, and reviewer rosters.
Only one plan may be open at once. A scored rubric is immutable: changing
criteria, weights, or score ranges requires a new round, preserving the
meaning of earlier evaluations.

Reviewers are added, edited, or retired rather than deleted. Retirement and
roster removal preserve evaluations, deactivate only future assignment
eligibility, and create audit history. Explicit review assignments carry
source (manual, automatic, or legacy), actor, Arbiter rule/trace, assignment
time, and non-destructive removal reason. The balanced assignment operation
backfills legacy score provenance, distributes work by current load, excludes
company conflicts through the review-governance policy, and is idempotent.
Managed plans require an active assignment for both organizer-entered and
signed-link reviewer scores. The organizer view exposes active assignments,
unfilled targets, conflicts, and recorded-score impact before roster changes.

### FB-2, FB-4, and FB-6: CFP lifecycle

An event owns named CFP variants with separately addressable public routes,
open/closed lifecycle state, attribution, routing, and a shared downstream
review/scheduling workflow. The form editor accepts only constrained
equals/show conditional rules, rejects chaining/unknown/locked targets, and
records versioned audited changes. Speakers can save signed, owner-bound
drafts and withdraw eligible proposals; withdrawal removes active review-plan
membership, cancels linked public sessions, retains historical evaluations,
and suppresses public exposure.

### Managed interaction contract

Interactive forms use GoSX `ActionForm`, managed `Form`, or the equivalent
explicit `data-gosx-form` protocol used by the agenda island. Successful
mutations use managed soft navigation and local result projection rather than
a document POST/refresh. This includes login magic-link request, workspace
import, task/review/communications operations, uploads, and the agenda drag
and unschedule flows. A repository test scans every `.gsx` source form and
fails if a raw form lacks the managed protocol.

## Exit condition for this sweep

This implementation and specification sweep is complete when this document,
the repository tests, and a clean-clone build are reviewed. The next required
action is operator-owned credential and infrastructure acceptance, beginning
with the selected Resend or SMTP transport. No repository visibility change is
part of that action.

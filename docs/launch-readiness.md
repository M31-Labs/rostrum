---
description: Evidence-based production-candidate checks for source, deployment, identity, providers, storage, and recovery.
nav_order: "06 / 07"
eyebrow: Prove the exact candidate
---

# Launch-readiness gate

<!-- markdownlint-disable MD013 -->

Rostrum contains a complete credential-free local workflow and the controls
needed to build a production candidate. A repository test pass is not a public
launch decision. The exact image, hosted configuration, identity, provider,
storage, and recovery paths still require operator-owned evidence.

## Decision summary

| Area | Repository posture | Required before production approval |
| --- | --- | --- |
| Source distribution | MIT license, evaluator quickstart, CI, community files, and versioned docs | Verify the exact candidate can be cloned anonymously and every public source/docs link resolves |
| Core program flow | Implemented and covered by unit, race, policy, and rendered-flow checks | Run the operator journey against the immutable candidate |
| Public read-only preview | Fail-closed `APP_MODE=demo` implementation and Kubernetes baseline | Prove the deployed host is running the intended seed/version and refusing every mutation |
| Identity | Organizer roles, one-time setup, magic link, OAuth adapters, passkeys, speaker/reviewer signed links | Test the selected sign-in method from an external browser/inbox |
| Email | Durable outbox plus Resend and SMTP transports | Send through the selected real transport and retain non-secret evidence |
| Canonical storage | JSON, SQLite WAL, and Postgres implementations share a validated aggregate contract | Restart, backup, and restore the exact selected backend |
| Accelevents | One-way published-program adapter with dry run and visible run ledger | Test a restricted event if the integration is enabled |
| Airtable | One-way, explicit, batched projection with dry-run and durable retry state | Test a restricted target base if the integration is enabled |
| Recovery | Checksummed online workspace import, pre-import backups, full archives, approved-upload bundle | Rehearse structured restore and stopped-process upload recovery |
| Release identity | `ROSTRUM_VERSION` is exposed by health and accepted by the container build | Record tag/SHA, image digest, health response, operator, and timestamp |

**Current decision:** suitable for a production-candidate deployment after the
repository gates pass. Not production-approved until every selected external
surface and the recovery runbook have acceptance evidence.

## Repository gates

Install the pinned tools:

```bash
go install m31labs.dev/gosx/cmd/gosx@v0.38.1
go install m31labs.dev/arbiter/cmd/arbiter@v1.9.0
```

Run:

```bash
make check
make smoke
make size-budget
```

These commands prove:

- Go and GoSX formatting, Arbiter policy validation, `go vet`, unit tests, and
  race tests pass for this source tree.
- A temporary `APP_MODE=demo` process boots with the deterministic seed;
  organizer, signed persona, CFP, public/embed/API/calendar, header, count,
  and mutation-refusal contracts pass.
- The production bundle stays within committed static HTML, island, runtime,
  server binary, distribution, and per-route client budgets.

They do not prove a remote host, credential, DNS/TLS setup, external database,
mailbox, Airtable base, backup system, or operator procedure.

## Implemented launch contract

### Program state and decisions

- A proposal can move through policy-backed routing, assigned human review,
  acceptance, speaker tasks, conflict-aware scheduling, publication, public
  JSON, calendar, embed, and export surfaces.
- Manual sessions retain their chosen format, speaker set, and duration while
  being scheduled and unscheduled.
- Event date changes rebase scheduled calendar days. Event timezone changes
  preserve each scheduled session's wall-clock time.
- Human review coverage and aggregates count attributed human evaluations.
  Governed final decisions enforce assignment/quorum policy, with an audited
  chair override path.

### Canonical storage

- JSON validates a cloned state and atomically replaces its file.
- SQLite uses WAL and a migration ledger.
- Postgres and `postgresql` use `DATABASE_URL` and the same aggregate contract.
- Rostrum remains canonical. Accelevents and Airtable are one-way projections
  and cannot alter review, scheduling, identity, archive, or audit state.

### Audit history

Every governed mutation records aggregate audit context and appends to an
operator-owned, fsynced, hash-chained JSON Lines ledger. Existing ledger
corruption fails startup instead of silently beginning a new chain.

The canonical transaction completes before the independent ledger append. If
the ledger filesystem fails after a state commit, Rostrum returns an error
that explicitly says the workspace commit succeeded. Preserve the evidence,
repair the ledger path, and reconcile that mutation. Do not describe this as a
distributed transaction.

### Export, import, and recovery

- Organizer/chair workspace export produces a checksummed, versioned envelope.
- Import validates export/schema version, checksum, and domain invariants,
  creates an exact pre-import backup, retains the newest ten, preserves local
  organizer principals and authentication records, and rebases upload paths.
- Full archive export streams workspace state, private uploads, and active and
  rotated audit segments.
- Approved-upload export produces a deterministic ZIP and digest-bearing
  manifest of approved regular files only, with path, symlink, count, and size
  defenses.
- Full upload recovery remains a stopped-process procedure. Source audit
  segments stay with the recovery record; they are not spliced into the
  receiving instance's chain.

### Public and identity edge

- Organizer, chair, and observer roles are application-enforced. Speaker and
  reviewer access is signed-link scoped.
- Public JSON emits only published sessions and their attached speakers.
- Production refuses weak/default session secrets, non-HTTPS public URLs, and
  in-memory persistence.
- The exact GoSX preflight is `gosx v0.38.1`; build/check targets reject a
  different version.
- `ROSTRUM_VERSION` is deployment-owned and visible at `/api/health`.

## Hosted preview acceptance

The preview must be a separate process, hostname, volume, audit path, and
session secret containing only the deterministic fictional seed. It must use:

```text
APP_MODE=demo
APP_ENV=production
PUBLIC_URL=https://demo.example.com
ROSTRUM_VERSION=<immutable-release-or-commit>
SESSION_SECRET=<unique-random-32+-character-secret>
SEED=demo
STORE_DRIVER=sqlite
DATA_PATH=/app/demo-data/rostrum.sqlite
AUDIT_LOG_PATH=/app/demo-data/audit.log
BACKUP_DIR=/app/demo-data/backups
DEMO_MODE=false
MAIL_DRIVER=outbox
ORGANIZER_EMAILS=
RESET_SECRET=
```

Do not provide external database, network mail, Accelevents, Airtable, or OAuth
credentials to the preview. Startup must fail if the seed fingerprint,
persistence path, release identity, or credential posture differs.

Verify all of the following against the deployed candidate:

1. `/api/health` returns `ok=true` and the expected immutable version.
2. `/api/v1/workspace` returns the M31 Systems Forum seed with non-zero
   published session/speaker counts.
3. `/organizer` is anonymously readable; the product tour, CFP, agenda, public
   agenda, gallery, and public calendar return 200.
4. `POST`, `PUT`, `PATCH`, and `DELETE` are refused. Authentication/setup,
   reset, import, export, upload, and private-file paths return 403.
5. The read-only banner and login explanation are visible.
6. Every response sends `X-Robots-Tag: noindex, nofollow, noarchive`.
7. No network provider request occurs.

Run the exact remote contract from a checkout of the same candidate:

```bash
make smoke \
  SMOKE_URL=https://demo.example.com \
  SMOKE_EXPECTED_VERSION=<immutable-release-or-commit>
```

Remote smoke checks the full anonymous read-only surface—including organizer
and signed speaker/reviewer persona routes—plus public/embed/API/calendar
output, deterministic counts, headers, mutation refusal, and exact version.
Manual browser inspection still verifies presentation quality.

## Provider acceptance

### Transactional email

Choose one transport for the live candidate:

- Resend: `MAIL_DRIVER=resend`, `RESEND_API_KEY`, and a verified `MAIL_FROM`.
- SMTP: `MAIL_DRIVER=smtp`, `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`,
  `SMTP_PASSWORD`, and `MAIL_FROM`.

From a real external inbox:

1. Submit a test proposal.
2. Verify arrival from the intended domain and a working signed portal link.
3. Repeat the trigger and confirm idempotency prevents duplicate lifecycle
   delivery.
4. Verify a scheduled-session message carries a valid calendar attachment.
5. Record provider message ID/time, recipient, release version, and result.
   Never record the token or SMTP password.

The demo outbox is useful product evidence, but it is not delivery proof.

### Accelevents (only when enabled)

Configure a restricted API credential and the target event URL only on the
live candidate. Rostrum publishes speakers attached to published, scheduled
sessions first, then those sessions; it never reads changes back.

1. Run the credential-free dry run and inspect the exact speaker/session plan.
2. Publish into a disposable or staging event and verify the expected six
   seeded public sessions (or the candidate workspace's published count).
3. Re-run and verify the provider updates stable Rostrum IDs rather than
   creating duplicates.
4. Confirm a draft or cancelled session does not reach the target event.
5. Preserve the visible run ledger result, timestamp, and release version;
   never preserve the API token.

### Airtable (only when enabled)

Use a restricted Personal Access Token with record-write access only to the
target base. Rostrum expects stable `Rostrum ID` fields and the columns listed
in [the deployment guide](deployment.md#airtable-projection).

1. Run the credential-free dry run and review its exact record plan.
2. Sync once and verify intended accepted speakers and scheduled sessions.
3. Sync again and verify upsert/update rather than duplicate records.
4. Change a harmless Airtable test row and prove Rostrum does not pull it back.
5. Check the visible sync ledger and independent audit history.

### Selected storage

- **JSON:** mount private durable storage for data, uploads, audit, and backups;
  keep one application replica.
- **SQLite:** boot and restart twice against the same path; retain the database,
  WAL files, uploads, audit, and backups together.
- **Postgres:** use a non-production endpoint, restart, submit/import a fixture,
  and prove state plus audit behavior. Complete the managed-database backup and
  restore drill separately.

## Release verification runbook

Complete every item against the exact immutable candidate. Attach command
output, timestamp, release identifier, image digest, and operator; never attach
secrets or signed speaker/reviewer tokens.

1. **Source:** record commit SHA and a clean, reviewed diff; from a logged-out
   environment, clone the repository and open the published documentation.
2. **Repository gates:** run `make check`, `make smoke`, and
   `make size-budget`.
3. **Package:** build the container with immutable `ROSTRUM_VERSION`; verify it
   runs as UID 10001 with read-only capabilities and durable mounts.
4. **Health and edge:** verify TLS, expected health version, cache headers,
   security headers, WebSocket upgrade, and public-data allow-list.
5. **Core journey:** submit; inspect routing trace; assign and review; make a
   governed decision; schedule; resolve a deliberate conflict; publish; verify
   agenda, gallery, API, and calendar.
6. **Identity:** test the selected organizer sign-in and signed reviewer and
   speaker links; verify observer and export restrictions.
7. **Recovery:** export workspace JSON, import into clean staging, verify local
   organizer access and pre-import backup, then rehearse archive upload restore
   with the process stopped.
8. **Providers:** complete selected email, storage, and optional
   Accelevents/Airtable acceptance.
9. **Preview:** complete every hosted-preview isolation check if a preview will
   be published.
10. **Approval:** record pass/fail, named owner, and any exception with expiry.

The [judging guide](judging-guide.md) is the presentation path. This document
is the operational go/no-go path.

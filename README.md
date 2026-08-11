# Rostrum

**Run your event's speaker program from one calm workspace.**

Rostrum is a self-hosted, open-source tool for event speaker programs. It
carries a proposal from an open call, through review and scheduling, to a
published agenda. Every routing and scheduling decision shows its rule and a
readable reason.

**Live demo:** [rostrum.m31labs.dev](https://rostrum.m31labs.dev). The seeded
event is M31 Systems Forum 2026. The demo leaves the organizer workspace open
so you can explore every surface.

## What Rostrum does today

- **Call for proposals (CFP).** A hosted public form collects proposals.
  Conditional questions appear only when they apply; for example, workshop
  logistics appear only for workshop proposals. A routing policy assigns each
  proposal a queue, an owner, and a track, and stores a readable trace.
- **Form builder.** Add, edit, reorder, and remove form fields from the
  organizer workspace. Open or close the call, set a close date the server
  enforces, and edit the success page.
- **Speaker portal.** Each submitter gets a portal through a signed link. No
  account, no password. Speakers update their profile, complete tasks, and
  upload slides and headshots. Uploads are capped at 10 MiB and limited to an
  allow-list of file types.
- **Multi-round review.** Each round has a weighted rubric. Reviewers open a
  signed review link and score their own assignments. An organizer can also
  record a score on a reviewer's behalf. Human scores alone set coverage and
  aggregates.
- **Conflict-aware agenda.** Drag sessions from an unscheduled bank onto the
  board, or use the keyboard move controls. Switch among list, day, week,
  track, and room views. Hard conflicts (a double-booked speaker or room)
  block publication. Warnings (same-track overlap) explain themselves.
- **Onboarding dashboard.** A live matrix shows each speaker's outstanding
  tasks. Uploads wait for organizer approval. The view refreshes over a
  WebSocket as speakers complete work.
- **Communications.** A template library with merge fields, a per-recipient
  preview, Gmail and Outlook compose links, a demo outbox, and a sent/queued
  ledger. Each submission creates a confirmation message carrying the
  speaker's portal link; configured Resend or SMTP delivery sends it as email.
  Each speaker has a private iCalendar file.
- **Publishing.** A public agenda with a personal itinerary stored on the
  visitor's device, a speaker gallery, embeddable widgets, public JSON, and a
  CSV (comma-separated values) export for organizers.
- **Accelevents export.** A one-way publisher sends speakers, then published
  sessions. A dry run needs no credential. A live sync needs an API key and
  records each run in a visible ledger.

## Try the live demo

1. Open the [public call](https://rostrum.m31labs.dev/submit/systems-forum-cfp)
   and submit a proposal. Choose the "Workshop" format to see a conditional
   question appear.
2. Follow the success page into your new speaker portal. Update your profile
   and complete a task.
3. Open the [organizer workspace](https://rostrum.m31labs.dev/organizer) and
   watch your submission arrive with its routing trace.
4. Open the [agenda](https://rostrum.m31labs.dev/organizer/agenda), drag a
   session into a conflict, and read the rule that blocks it.
5. End at the [public agenda](https://rostrum.m31labs.dev/public/m31-systems-forum-2026/agenda)
   and the [speaker gallery](https://rostrum.m31labs.dev/public/m31-systems-forum-2026/speakers).

## Quickstart

Requirements: Go 1.26 or newer. No database, no Node toolchain, no external
credential.

```bash
git clone https://github.com/m31-labs/rostrum
cd rostrum
cp .env.example .env
go run .
```

Open [http://localhost:8080](http://localhost:8080). The first run creates
`data/rostrum.json` with a fully seeded demo event. For a disposable
in-memory workspace that resets on restart:

```bash
DEMO_MODE=memory go run .
```

Entry points:

- Organizer workspace: [http://localhost:8080/organizer](http://localhost:8080/organizer)
- Public CFP: [http://localhost:8080/submit/systems-forum-cfp](http://localhost:8080/submit/systems-forum-cfp)
- Public agenda: [http://localhost:8080/public/m31-systems-forum-2026/agenda](http://localhost:8080/public/m31-systems-forum-2026/agenda)
- Speaker portal: submit a proposal; the confirmation email and the success
  page both carry a signed link into your portal.

In the organizer workspace, press `Ctrl/Cmd K` for the search switcher, or
press `G` plus a route key to jump to any surface.

## How a program moves through Rostrum

1. **Submit.** A speaker completes the public form. The routing policy
   assigns a queue, an owner, and a track. Rostrum sends a confirmation email
   and opens the speaker's portal.
2. **Review.** Organizers run one or more weighted rubric rounds. Reviewers
   score through signed links; scores stay attributed to their author.
3. **Schedule.** Organizers drag accepted sessions onto the agenda. Hard
   conflicts block publication until an organizer resolves them.
4. **Publish.** The committed schedule feeds the public agenda, the speaker
   gallery, portals, calendar files, the JSON API, embeds, and the
   Accelevents export. One record drives all of them.

## Built for self-hosters

- **Passwordless access.** Speakers and reviewers authenticate with signed,
  expiring links. There are no passwords to store, reset, or leak.
- **Decisions you can defend.** Routing and scheduling rules live in
  versioned policy files under `rules/`. Every decision keeps its rule name
  and a human-readable reason.
- **Your program in one file.** The whole workspace lives in one JSON file
  with validated, atomic writes. SQLite (WAL) and Postgres use the same
  aggregate contract when an operator needs a database backend.
- **Independent audit history.** Every durable workspace mutation is also
  appended and fsynced to an operator-owned JSON Lines ledger, so a workspace
  restore cannot rewrite the operational history that preceded it.
- **One binary.** A single Go process serves every page, the live dashboard,
  and the API. No database and no background workers.
- **A careful public edge.** Public JSON serves only published sessions and
  their speakers. It never includes email addresses, proposal text, review
  data, or drafts. Rate limits guard the public form.

## Identity model

- Speakers hold signed, expiring portal links. A valid link binds the browser
  session to that one speaker.
- Reviewers hold signed review links that an organizer copies from the Review
  page. Each link authenticates one reviewer on every request.
- Rostrum enforces its own organizer roles: `organizer`, `chair`, and
  `observer`. Configure `ORGANIZER_EMAILS` before first boot, or use the
  one-time `/setup` break-glass link to establish the first organizer.
  Organizers can sign in with a magic link, configured GitHub or Google OAuth,
  or a registered passkey. See [docs/deployment.md](docs/deployment.md).

## Public interfaces

| Endpoint | Purpose | Cache |
|---|---|---|
| `GET /api/health` | Application name, version, timestamp | 30 seconds |
| `GET /api/v1/workspace` | Public discovery links and counts | 60 seconds |
| `GET /api/v1/schedule` | Published sessions only | 60 seconds |
| `GET /api/v1/speakers` | Speakers on published sessions only | 60 seconds |
| `GET /calendar/{speaker}.ics` | Private per-speaker calendar file | private, 60 seconds |
| `GET /organizer/export/submissions.csv` | Organizer CSV export | private, no-store |
| `GET /organizer/export/workspace.json` | Checksummed organizer workspace export | private, no-store |
| `GET /organizer/export/archive.tar.gz` | Workspace, uploads, and audit archive | private, no-store |

## Optional integrations

Rostrum runs complete with zero credentials. Add these when you want them:

- **Transactional email.** Set `MAIL_DRIVER=resend` with `RESEND_API_KEY`, or
  set `MAIL_DRIVER=smtp` with `SMTP_HOST`, to send real confirmation mail.
  SMTP remains a standards-based self-hosted option; without a configured
  driver, messages record to a visible demo outbox.
- **Accelevents.** Set `ACCELEVENTS_API_KEY` and `ACCELEVENTS_EVENT_URL` to
  unlock live publishing. Dry runs work without either.
- **Airtable.** Set `AIRTABLE_PAT` and `AIRTABLE_BASE_ID` to enable a
  one-way, batched upsert projection. Rostrum remains the canonical store;
  Airtable receives accepted-speaker and scheduled-session records keyed by
  stable `Rostrum ID` values. Dry runs work without a token.
- **Storage.** Set `STORE_DRIVER=sqlite` for a local WAL-backed database, or
  `STORE_DRIVER=postgres` with `DATABASE_URL` for Postgres. Set
  `AUDIT_LOG_PATH` when the independent audit ledger should live outside the
  default `data/` directory, and `BACKUP_DIR` to locate the newest-ten
  pre-import backups.

See [.env.example](.env.example) for every setting.

## Scope today

One Rostrum instance runs one organization's program: one event workspace and
one process. Organizer, chair, and observer roles are scoped to that single
workspace; this release has no multi-tenant account model.

## Development

Install the GoSX CLI and Arbiter for the full check suite:

```bash
go install m31labs.dev/gosx/cmd/gosx@v0.38.1
go install m31labs.dev/arbiter/cmd/arbiter@v1.9.0
```

Then run:

```bash
make check          # gofmt, template format, policy validation, vet, tests, race
make build          # production bundle in dist/
make smoke          # rendered HTML smoke test against a temporary local server
make release-check  # check plus the committed size budgets
```

## Technical notes

- Rostrum is built with GoSX, a server-rendered Go component framework. Every
  route renders complete HTML and works without JavaScript. The application
  ships no bespoke browser JavaScript of its own.
- Scheduling and routing rules are Arbiter policy files in `rules/`; the UI
  shows each fired rule and its reason.
- The build enforces committed size budgets ([size-budget.json](size-budget.json))
  and runtime budgets ([perf-budget.json](perf-budget.json)).

## Deploy

The included Dockerfile packages the verified `dist/` bundle and runs as a
non-root user. See [docs/deployment.md](docs/deployment.md) for the container
run command, the persistent volume, production secret requirements, and the
reverse-proxy boundary.

The current release gate, external-provider setup, and intentionally deferred
follow-up work are recorded in [docs/launch-readiness.md](docs/launch-readiness.md).

## License

MIT. See [LICENSE](LICENSE).

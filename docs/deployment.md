---
description: Build, configure, secure, persist, back up, and operate one Rostrum event workspace.
nav_order: "05 / 07"
eyebrow: Operate one durable workspace
---

# Deployment guide

<!-- markdownlint-disable MD013 -->

Rostrum deploys as one Go process plus the `app/` templates and `public/`
assets. One replica against one JSON data file serves a single-organization
instance.

For evaluation rather than operations, start with the
[judge and organizer guide](judging-guide.md). This document assumes an
operator is preparing a persistent live or read-only deployment.

## Build the production bundle

Requirements:

- Go 1.26 or newer.
- The GoSX CLI: `go install m31labs.dev/gosx/cmd/gosx@v0.38.1`
- The Arbiter CLI (used by `make check`): `go install m31labs.dev/arbiter/cmd/arbiter@v1.9.0`

```bash
make check
make build
make smoke
```

`make build` writes the `dist/` bundle: the static server binary at
`dist/server/app`, the route templates, framework runtime assets, public
files, and seeded data. The packaging step removes sourcemaps and Go source
that production does not need. Mount a writable directory for `DATA_PATH`
and for uploaded files under `data/uploads`.

## Container

```bash
make build
docker build --build-arg ROSTRUM_VERSION="$(git rev-parse --short HEAD)" -t rostrum:local .
docker run --rm -p 8080:8080 \
  -e APP_ENV=production \
  -e PUBLIC_URL=https://program.example.com \
  -e SESSION_SECRET='replace-with-a-random-secret-of-at-least-32-characters' \
  -e DATA_PATH=/app/data/rostrum.json \
  -v rostrum-data:/app/data \
  rostrum:local
```

The Dockerfile copies the verified `dist/` bundle. The image contains no Go
source and runs as the non-root `rostrum` user (UID 10001). In production
the process refuses to start with:

- the development session secret, or any secret shorter than 32 characters;
- a `PUBLIC_URL` that is not HTTPS;
- the in-memory store (`DEMO_MODE=memory`).

Set `ROSTRUM_VERSION` to the immutable release tag or commit SHA. The value is
returned by `GET /api/health`, so a deployment record can prove which Rostrum
build is serving. Check every live deployment directly:

```bash
curl -fsS https://program.example.com/api/health
```

Perform the authenticated live organizer workflow in the launch checklist
separately. `make smoke SMOKE_URL=…` is the deterministic `APP_MODE=demo`
contract and is expected to fail against an organizer-gated live deployment.

## Hosted read-only preview

The public preview uses a separate process, subdomain, volume, and release
configuration from every live organizer deployment. Set the explicit posture
below; `APP_MODE=demo` is fail-closed and does not turn an ordinary live
instance into a demo:

```text
APP_MODE=demo
APP_ENV=production
PUBLIC_URL=https://demo.example.com
ROSTRUM_VERSION=2026.08.11-<immutable-commit>
SESSION_SECRET=<unique-random-secret-at-least-32-characters>
SEED=demo
STORE_DRIVER=sqlite
DATA_PATH=/app/demo-data/rostrum.sqlite
MAIL_DRIVER=outbox
ORGANIZER_EMAILS=
RESET_SECRET=
```

Use only fictional seeded data. The demo refuses `DATABASE_URL`, the legacy
`DEMO_MODE=memory` setting, relative or in-memory paths, a mutable `dev`
release identity, Resend/SMTP/Accelevents/Airtable credentials, OAuth
credentials, and a network mail driver. The demo seed is built with a
deterministic fixture timestamp, and startup compares a full state fingerprint
before serving: any changed speaker, proposal, review record, schedule,
resource, integration, communication, audit, or sync record fails closed.
Keep the demo volume and audit path separate from the live volume; never reuse
a production `DATA_PATH`, upload directory, or session secret. If a demo
volume is ever changed or copied from live, redeploy it as a fresh canonical
fixture instead of repairing it in place.

The demo exposes public pages, signed-link read surfaces, `/organizer/*`, and
`/live` for inspection without an organizer session. Every unsafe HTTP method,
authentication/setup route, reset, import, export, and upload is refused with
403. A store-level read-only wrapper remains in place even if a future route
forgets the middleware. The UI labels the workspace as read-only, the login
page explains that sign-in is disabled, and every response sends
`X-Robots-Tag: noindex, nofollow, noarchive`. The local quickstart remains the
place to exercise interactive mutations. A process-local client-IP limiter
also caps anonymous preview traffic; keep the reverse proxy's normal rate
limits enabled for a public deployment.

For Kubernetes, start from [`deploy/k8s/rostrum-demo.yaml`](https://github.com/M31-Labs/rostrum/blob/main/deploy/k8s/rostrum-demo.yaml).
It creates a separate namespace, volume, service, ingress, and session secret.
Replace only its documented image, host, ingress, and secret placeholders.

Before sharing the preview, run the acceptance checks in
[launch readiness](launch-readiness.md#hosted-preview-acceptance). A hosted
URL is a convenience surface, not a substitute for repository or local-run
evidence.

Verify the exact read-only release from a checkout of the same candidate:

```bash
make smoke \
  SMOKE_URL=https://demo.example.com \
  SMOKE_EXPECTED_VERSION=<immutable-release-or-commit>
```

Remote smoke covers the complete demo contract: organizer and signed
persona inspection, public/embed/API/calendar surfaces, deterministic counts,
no-index headers, mutation refusal, and the exact expected version.

## Reverse proxy

Terminate TLS (Transport Layer Security) at a trusted reverse proxy and
preserve WebSocket upgrades for `/live`. Rostrum permits other sites to
frame its `/public/*` pages, because that is how the embeddable agenda
works; every other route sends `frame-ancestors 'none'`. Do not add a
blanket `X-Frame-Options` header. If your proxy replaces the application
CSP (Content Security Policy), keep the policy route-aware.

## Identity and access

Rostrum enforces organizer identity itself; it needs no identity-aware
proxy in front of `/organizer`. Sign-in happens at `/login`, which offers
every configured method: an emailed magic link, GitHub, Google, and a
registered passkey.

- `/organizer/*` requires a session carrying the `organizer`, `chair`, or
  `observer` role. An anonymous visitor is redirected to
  `/login?next=<path>`; a JSON request gets 401.
- `/organizer/export/submissions.csv` requires `organizer` or `chair`
  specifically. `observer` reaches the rest of `/organizer` but never this
  export, because it carries speaker PII (personally identifiable
  information). A cookie-less request gets 403, never a redirect.
- `/portal/*`, `/calendar/*`, and `/portal-file/*` require a signed speaker
  link or a bound session.
- `/review/{token}` requires a signed reviewer link on every request.

### Organizer allowlist

Set `ORGANIZER_EMAILS` to a comma-separated list of addresses. A magic-link
or OAuth (Open Authorization) sign-in from one of these addresses is
granted the organizer role and recorded as a workspace Principal, so the
grant survives a restart even if you later change the allowlist.

### Break-glass bootstrap

A fresh self-host with `ORGANIZER_EMAILS` empty and no stored organizer logs
a one-time setup URL at startup:

```text
Rostrum has no organizer yet. Finish setup at: https://program.example.com/setup?token=...
```

Open that URL, enter your email and name, and Rostrum creates the first
organizer Principal. The token is single-use and process-wide: a restart
with an existing organizer never re-arms it, and `/setup` returns 404 once
the token is spent. Prefer `ORGANIZER_EMAILS` for a deployment you can
configure before first boot; break-glass covers the one you cannot.

### OAuth providers

Set both variables in a pair to show that provider's button on `/login`.
Each provider's redirect URL derives from `PUBLIC_URL`:

- GitHub: `AUTH_GITHUB_CLIENT_ID`, `AUTH_GITHUB_CLIENT_SECRET`. Redirect URL
  `{PUBLIC_URL}/auth/oauth/github/callback`.
- Google: `AUTH_GOOGLE_CLIENT_ID`, `AUTH_GOOGLE_CLIENT_SECRET`. Redirect URL
  `{PUBLIC_URL}/auth/oauth/google/callback`.

### Passkeys

Passkey sign-in (`/auth/webauthn/*`) needs no configuration beyond
`PUBLIC_URL`: it is the WebAuthn (Web Authentication) origin the browser's
passkey ceremony verifies against. Register a passkey from a signed-in
organizer session.

### Storage note for the identity plane

Issued magic-link tokens and registered passkeys persist in the same canonical
workspace aggregate as everything else, so they follow the single-replica rule
below. A future multi-instance deployment needs validated shared behavior
behind the same `MagicLinkStore` and `WebAuthnStore` interfaces.

Keep these routes reachable by the public: `/`, `/login`, `/submit/*`,
`/public/*`, `/api/health`, `/api/v1/*`, `/portal/*`, and `/review/*`. The
last two must stay reachable because speakers and reviewers open their
signed links from an inbox.

`POST /demo/reset` restores the seeded workspace. Set `RESET_SECRET` to
require a matching secret. In production with no secret set, reset is
disabled entirely.

## Storage and replicas

The JSON store validates a cloned next state, then replaces the data file
with an atomic rename. SQLite uses the same aggregate contract with WAL;
Postgres uses the configured `DATABASE_URL`. Run exactly one application
replica against one workspace volume until a deployment is deliberately
configured around a shared Postgres backend.

Keep `DATA_PATH`, `data/uploads`, and `AUDIT_LOG_PATH` on durable
operator-owned storage. The independent audit ledger is intentionally outside
the mutable workspace state: a workspace restore never rewrites the active
operational history.

## Export, backup, and restore

Organizers and chairs (never observers) can use **Settings → Export and
restore**. Every export writes an audit event and returns `Cache-Control:
private, no-store`.

- **Workspace export** is a checksummed JSON envelope containing the complete
  structured program state. Transient magic-link records are excluded.
- **Workspace restore** validates the export version, schema version,
  checksum, and domain invariants before changing state. It writes an exact
  pre-import backup to `BACKUP_DIR` (default `data/backups`) and retains the
  newest ten. The receiving host keeps its own organizer principals,
  passkeys, and pending magic links, so an import cannot lock out its local
  operator. Imported upload references are rebased to that host’s
  `data/uploads` directory.
- **Full archive** is a streaming `.tar.gz` containing `workspace.json`, all
  regular files from `data/uploads`, and every active or rotated audit-log
  segment. It is the cold-storage and file-recovery artifact.

Full-archive file recovery is deliberately a stopped-process operation in
this release. Extract an archive only into a new, private staging directory;
first restore its `workspace.json` through the validated Settings flow, then
stop Rostrum and copy the staged `uploads/` files into the receiving
`data/uploads` directory. Restart and verify portal downloads byte-for-byte.
Keep the staged `audit/` directory with the recovery record; do not splice it
into a live audit log, whose hash chain belongs to the receiving instance.
This preserves both the source evidence and the destination’s audit history.

## Accelevents publishing

Rostrum publishes a deliberately narrow, one-way program projection to
Accelevents. Rostrum remains canonical: the adapter never reads Accelevents
changes back into the workspace. Configure a restricted staging event first:

```text
ACCELEVENTS_EVENT_URL=https://...
ACCELEVENTS_API_KEY=...
```

The Integrations page always offers a credential-free dry run. Live publishing
is separately locked until both variables are present, then sends only
speakers attached to published, scheduled sessions, followed by those
published sessions. It records a complete or failed run in Rostrum's visible
sync ledger. Use the dry run for
repository evaluation; before production, repeat the publish against a
disposable or restricted Accelevents event and verify the resulting speakers,
sessions, and stable Rostrum identifiers directly in that event.

This adapter is an explicit operator action rather than background
synchronization. A remote rejection stops the run and preserves its failure in
the ledger; correct the provider configuration and publish again. Do not point
the hosted read-only preview at Accelevents credentials.

## Airtable projection

Rostrum treats Airtable as a one-way operational projection, never as the
transactional program database. Keep `STORE_DRIVER=json`, `sqlite`, or
`postgres` as the canonical source, then configure:

```text
AIRTABLE_PAT=...
AIRTABLE_BASE_ID=app...
AIRTABLE_SPEAKERS_TABLE=Speakers
AIRTABLE_SESSIONS_TABLE=Sessions
```

Use an Airtable Personal Access Token scoped only to the target base with
record-write access. Airtable API keys are retired; do not add a legacy key to
the environment. The connector makes batched `PATCH` requests with
`performUpsert` keyed on the `Rostrum ID` field, so replay after a process
failure is safe. Create these fields in both tables before enabling live sync:

| Table | Required fields |
| --- | --- |
| Speakers | `Rostrum ID`, `Rostrum Schema`, `Name`, `Email`, `Role`, `Company`, `Biography`, `Website`, `LinkedIn` |
| Sessions | `Rostrum ID`, `Rostrum Schema`, `Title`, `Description`, `Starts At`, `Ends At`, `Room`, `Track`, `Speaker IDs` |

The Integrations page offers a credential-free dry run and an explicit
**Sync Airtable now** control. It queues changed records in the canonical
outbox before network delivery, batches ten records per request, stays below
the five-requests-per-second base limit, stops on the first remote error, and
uses durable backoff for failed records. Rostrum does not upload speaker files
as Airtable attachments and does not pull any Airtable changes back.

## Operational configuration

- Use a secret manager for session, mail, OAuth, database, Accelevents, and
  Airtable credentials.
- Restrict outbound egress to the provider APIs you configure.
- Poll `GET /api/health`; it returns the application name, version, and
  timestamp.
- Keep sync history and review provenance as audit data.
- Set the proxy body-size limit to 12 MiB or more. Rostrum accepts uploads
  to 10 MiB inside a 12 MiB request envelope and caps all other request
  bodies at 1 MiB.
- Public form submissions are rate limited per session and per IP address.
  The limiters are in-memory and reset on restart.

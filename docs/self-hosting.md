---
description: Install, bootstrap, secure, back up, upgrade, and operate a production Rostrum workspace.
nav_order: "05 / 08"
eyebrow: Self-host one speaker program
---

# Self-hosting manual

<!-- markdownlint-disable MD013 MD049 -->

This is the start-to-finish operator path for a real event. It turns a clean
source checkout into one durable Rostrum workspace, creates the first
organizer, and defines the checks to keep that workspace recoverable.

Rostrum operates one organization's event workspace per instance. The
recommended first production topology is deliberately small:

```text
Internet → TLS reverse proxy → one Rostrum process → one durable data volume
                                             └── optional SMTP or Resend
```

Use the [deployment reference](deployment.md) after this manual when you need
the Kubernetes manifests, external publishing adapters, or the exact hosted
observer posture. Use [launch readiness](launch-readiness.md) before accepting
real proposals.

## Choose the installation shape

| Path | Choose it when | Persistent boundary |
| --- | --- | --- |
| Container | You already operate Docker, Podman, Kubernetes, or another OCI runtime | Mount all of `/app/data` |
| Production bundle | You want a native service with no container runtime | Keep the application root's `data/` directory durable |
| Source run | You are evaluating or developing, not operating a long-lived public service | The checkout's `data/` directory |

The container path is the least surprising production installation. The image
runs as numeric UID and GID `10001`, and its fixed `/app/data` mount contains
the canonical store, private uploads and approved public portraits, audit
ledger, and import backups together.

Whatever path you choose, run one Rostrum process against one workspace. JSON
and SQLite are intentionally single-process choices. Postgres coordinates the
canonical workspace row, but uploads and the independent audit ledger still
need deliberate shared storage and operating procedures; selecting Postgres
alone is not authorization to add replicas.

## Prerequisites

For a source checkout and verified production build, install:

- Git.
- Go 1.26, matching [`go.mod`](https://github.com/M31-Labs/rostrum/blob/main/go.mod).
- GNU Make or a compatible `make` implementation.
- A POSIX shell and `curl` for the repository checks.
- GoSX `v0.38.1`.
- Arbiter `v1.9.0` for policy validation in `make check`.
- TinyGo `v0.40.1` and Binaryen `wasm-opt` version `125` for the production
  WebAssembly islands emitted by `make build`.
- An OCI container runtime only if you choose the container path.
- A TLS-capable reverse proxy for every internet-facing installation.

The exact Linux tool bootstrap, checksums, and separate Go SDK used for
TinyGo compatibility are recorded in the repository's
[`CI` workflow](https://github.com/M31-Labs/rostrum/blob/main/.github/workflows/ci.yml).
CI currently points `GOSX_TINYGO_GOROOT` at Go 1.25.9 while the application
itself builds with the Go version declared by `go.mod`. Follow that pattern if
your TinyGo installation does not accept the application SDK directly.

Install the pinned build tools, then clone and verify the exact source you plan
to operate:

```bash
go install m31labs.dev/gosx/cmd/gosx@v0.38.1
go install m31labs.dev/arbiter/cmd/arbiter@v1.9.0

tinygo version
wasm-opt --version

git clone https://github.com/M31-Labs/rostrum.git
cd rostrum
git rev-parse HEAD
make check
make build
```

Record the full commit SHA from `git rev-parse HEAD`. Use that same immutable
value as `ROSTRUM_VERSION` and as the container tag or release identifier.
`GET /api/health` reports it later.

`make build` writes the release output under `dist/`. Its deployable pieces are
the server binary, route templates, generated runtime assets, public assets,
and build metadata. The production process needs those pieces as a unit;
copying only `dist/server/app` produces an incomplete installation. Do not
deploy `dist/data`: it is build-time prerender state, not a production seed.

## Prepare production configuration

Rostrum reads process environment first and then the standard `.env` files in
its resolved application root. A service manager or secret manager is the
safer production source. Do not commit a populated `.env` file.

The file sequence is `.env`, `.env.local`, `.env.<mode>`, then
`.env.<mode>.local`; later files refine earlier files, while variables already
present in the process environment stay locked. `APP_ENV` selects the mode.

Start with this minimum live posture. `REPLACE_ME` is deliberately too short
to pass production startup; generate a real secret before first boot.

```dotenv
APP_ENV=production
APP_MODE=live
ROSTRUM_VERSION=<full-release-tag-or-commit-sha>
PORT=8080
PUBLIC_URL=https://program.example.com
SESSION_SECRET=REPLACE_ME

INITIAL_WORKSPACE=fresh
STORE_DRIVER=sqlite
DATA_PATH=/app/data/rostrum.sqlite
AUDIT_LOG_PATH=/app/data/audit.log
BACKUP_DIR=/app/data/backups
UPLOAD_DIR=/app/data/uploads

MAIL_DRIVER=outbox
MAIL_FROM=Rostrum <noreply@example.com>
ORGANIZER_EMAILS=
```

For a native bundle, replace the `/app/data/...` paths with absolute paths
under that bundle's persistent `data/` directory. Keep
`INITIAL_WORKSPACE=fresh` explicit: it creates one placeholder event and one
editable open call for proposals, but no fictional speakers, proposals,
reviews, or sessions. `INITIAL_WORKSPACE=empty` creates only the event
skeleton. Initialization runs only when the configured store is empty;
changing it alone does not rewrite an existing workspace.

The `fresh` starter CFP is open by design. Keep public ingress disabled during
bootstrap, then review its dates, fields, categories, copy, and mail behavior
before sharing the submission URL. Choose `INITIAL_WORKSPACE=empty` when even
a placeholder intake route must not exist on first boot.

Generate `SESSION_SECRET` with a cryptographically secure secret generator
available in your platform or secret manager. Production refuses the
development value, a value shorter than 32 characters, an HTTP `PUBLIC_URL`,
in-memory storage, or the build-only static-export bypass. A non-local
`PUBLIC_URL` enforces the same safeguards even when `APP_ENV` was accidentally
left in development.

### Why the first configuration uses the outbox

`MAIL_DRIVER=outbox` performs no network delivery. It is safe for bootstrap,
but it is not a production email service and must not be used as evidence that
a recipient received mail. With no real transport, the one-time setup flow
signs the first organizer's browser in directly. Configure and acceptance-test
SMTP or Resend before opening a call that depends on confirmations, magic
links, reminders, or speaker portal delivery.

## Install with a container

The repository Dockerfile packages the already verified `dist/` bundle; build
the bundle before the image:

```bash
rostrum_revision="$(git rev-parse HEAD)"
docker build \
  --build-arg ROSTRUM_VERSION="$rostrum_revision" \
  -t "rostrum:$rostrum_revision" .
```

The Dockerfile deliberately excludes `dist/data` and creates an empty
`/app/data` boundary. The configured `INITIAL_WORKSPACE` or pinned
`INITIAL_WORKSPACE_PATH` therefore controls first boot; a build machine's
prerender state cannot become the container's workspace.

Store the production variables in a root-readable environment file or inject
them from your runtime's secret manager. Create a durable volume, then start
exactly one container:

```bash
docker volume create rostrum-data
docker run -d \
  --name rostrum \
  --restart unless-stopped \
  --env-file /secure/path/rostrum.env \
  -p 127.0.0.1:8080:8080 \
  -v rostrum-data:/app/data \
  "rostrum:$rostrum_revision"
```

Binding to loopback assumes the reverse proxy runs on the same host. On a
container network, expose port 8080 only to that private proxy network. Never
publish the application port directly to the internet as a substitute for
TLS.

The volume must be writable by UID/GID `10001`. Mounting only the SQLite or
JSON file is insufficient: `UPLOAD_DIR` (including any approved public
portraits), the audit segments, and the automatic pre-import backups are part
of the recovery boundary.

## Install the native production bundle

Use a dedicated, unprivileged operating-system account and a stable
application root such as `/opt/rostrum`. Install these paths from `dist/`:

```text
dist/app/                → /opt/rostrum/app/
dist/assets/             → /opt/rostrum/assets/
dist/public/             → /opt/rostrum/public/
dist/server/             → /opt/rostrum/server/
dist/build.json          → /opt/rostrum/build.json
dist/gosx-grammar.blob   → /opt/rostrum/gosx-grammar.blob
```

Create `/opt/rostrum/data/uploads` separately and make the entire
`/opt/rostrum/data` tree durable and writable by the service account. Do not
replace it with `dist/data` during an upgrade. Set these native paths:

```dotenv
GOSX_APP_ROOT=/opt/rostrum
DATA_PATH=/opt/rostrum/data/rostrum.sqlite
AUDIT_LOG_PATH=/opt/rostrum/data/audit.log
BACKUP_DIR=/opt/rostrum/data/backups
UPLOAD_DIR=/opt/rostrum/data/uploads
```

Install the following baseline as `/etc/systemd/system/rostrum.service`. Put
secrets in the referenced environment file with restrictive permissions; do
not paste them into the unit.

```ini
[Unit]
Description=Rostrum speaker-program workspace
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=rostrum
Group=rostrum
WorkingDirectory=/opt/rostrum
EnvironmentFile=/etc/rostrum/rostrum.env
ExecStart=/opt/rostrum/server/app
Restart=on-failure
RestartSec=5s
UMask=0077
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
ProtectSystem=strict
ReadWritePaths=/opt/rostrum/data

[Install]
WantedBy=multi-user.target
```

Reload the service manager, enable the service, and follow first-boot logs:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now rostrum.service
sudo journalctl --unit rostrum.service --follow
```

The first boot may print the one-time organizer setup URL described next.

## Bootstrap the first organizer

There are two supported first-identity paths.

### Bootstrap without working email

1. Leave `ORGANIZER_EMAILS` empty.
2. Start Rostrum against a new `INITIAL_WORKSPACE=fresh` or
   `INITIAL_WORKSPACE=empty` store.
3. Read the process log once. It contains a URL shaped like:

   ```text
   Rostrum has no organizer yet. Finish setup at: https://program.example.com/setup?token=...
   ```

4. Open that exact HTTPS URL in the organizer's browser.
5. Enter the organizer name and email address.
6. With no real mail transport configured, Rostrum creates the first organizer
   principal and signs that browser in directly.

The setup token is random, held only as a hash in process memory, and consumed
once. Treat the logged URL as a secret. Once a stored organizer exists, a
restart does not arm another setup token.

### Bootstrap with tested email or OAuth

If a real mail transport is already acceptance-tested, set
`ORGANIZER_EMAILS` to a comma-separated, case-insensitive allowlist and use
the magic-link form at `/login`. Alternatively, configure both variables in a
GitHub or Google OAuth pair and use that provider without mail. OAuth still
requires a verified provider identity. For GitHub specifically, you can use a
handle allowlist instead of putting private GitHub email addresses in config:

```dotenv
AUTH_GITHUB_HANDLES=octocat,program-chair
```

Handles are matched case-insensitively against GitHub's verified `login`
profile field. Rostrum also requests the `user:email` scope and requires a
verified account email so the accepted sign-in can be stored as a durable
Principal. The handle list is a bootstrap allowlist: once accepted, the
durable email Principal governs future access and role reconciliation. Remove
access explicitly with `PRINCIPAL_ROLES=email=none`, rather than relying on
removing a handle from the bootstrap list.

Do not configure an allowlist on a fresh host with only the outbox transport:
the allowlist suppresses break-glass setup, while the outbox cannot deliver a
sign-in link. Passkeys are registered only after a successful organizer
sign-in.

An allowlisted sign-in is persisted as an organizer principal. Removing that
address from `ORGANIZER_EMAILS` later does **not** revoke the stored principal.

For explicit organizer-surface roles, use `PRINCIPAL_ROLES`:

```dotenv
PRINCIPAL_ROLES=owner@example.com=organizer+chair,program-chair@example.com=chair,auditor@example.com=observer
```

Only `organizer`, `chair`, and `observer` are accepted. Entries are strict,
case-insensitive email addresses; duplicate addresses, duplicate/unknown roles,
or a non-empty mapping that leaves no organizer stop startup atomically. Each
listed principal receives exactly the listed roles, while omitted durable
principals remain unchanged. Use `former@example.com=none` to retain an
explicit durable deny record and revoke every organizer-surface role; this
also prevents the legacy allowlist from restoring access. Applying a changed
mapping at restart is audited. Every request reconciles organizer authority
against the durable principal, so a demotion or revocation neutralizes stale
session, pending magic-link, and passkey role payloads. Speaker/reviewer roles
remain scoped independently. Provider-supplied role claims are never trusted.
The UI intentionally has no principal editor, so use deployment configuration
plus a verified restart for grants, demotions, or revocations, and keep at
least one tested organizer recovery path.

## Runtime configuration reference

Set production values explicitly rather than depending on development
fallbacks.

### Process and deployment

| Variable | Production meaning |
| --- | --- |
| `APP_ENV` | `development` or `production`; unknown values fail startup. Set `production` explicitly. HTTPS, a strong session secret, durable `DATA_PATH`, and a disabled static-export bypass are also mandatory whenever `PUBLIC_URL` is non-local. |
| `APP_MODE` | `live` (the default) for a real organizer workspace; `preview` for a generic, fail-closed, anonymous observer deployment initialized from a pinned template. |
| `ROSTRUM_VERSION` | Immutable release tag or commit SHA returned by `/api/health`. Do not deploy `dev`. |
| `PORT` | HTTP listen port; defaults to `8080`. Terminate TLS at the reverse proxy. |
| `PUBLIC_URL` | Exact external HTTPS origin, such as the `program.example.com` origin shown in this manual. The development fallback is the HTTP loopback origin on `PORT`. It drives canonical links, secure cookies, magic links, OAuth callbacks, and WebAuthn origin checks. A path-prefix deployment is not documented; use a dedicated origin. |
| `SESSION_SECRET` | Unique secret of at least 32 characters. It protects organizer sessions and signed speaker/reviewer links; the development fallback is refused in production and on a non-local origin. |
| `GOSX_APP_ROOT` | Optional explicit application-bundle root. Useful for native installs; the container resolves `/app` automatically. |
| `RESET_SECRET` | Guards `POST /workspace/reset` when configured. Leave it empty on a real production workspace; production disables an unguarded reset. |
| `TRUSTED_PROXY_CIDRS` | Comma-separated exact proxy networks allowed to supply `X-Forwarded-For`. Empty ignores forwarded headers. Invalid CIDRs stop startup; never use a trust-all network. |
| `ORGANIZER_EMAILS` | Backward-compatible comma-separated organizer bootstrap allowlist. A listed email is persisted as an organizer after successful magic-link or OAuth verification. |
| `PRINCIPAL_ROLES` | Strict deployment-owned role map using `email=role+role,...`. Supports organizer, chair, and observer; use `email=none` for explicit revocation. Listed roles are authoritative and a non-empty map must retain an organizer. |

Changing `SESSION_SECRET` signs out organizer sessions and invalidates existing
signed speaker and reviewer links. Plan to reissue those links after an
intentional rotation.

### Initial state and persistence

| Variable | Production meaning |
| --- | --- |
| `INITIAL_WORKSPACE` | `fresh` (the production default) for a starter CFP, or `empty` for only an event skeleton. It applies only when no workspace exists. |
| `INITIAL_WORKSPACE_PATH` | Optional path to a raw, validated `domain.State` JSON template. A relative path resolves under the application root. Preview mode requires this path. |
| `INITIAL_WORKSPACE_SHA256` | Optional 64-character hexadecimal SHA-256 pin over the exact template bytes. Preview mode requires this or the file form below. |
| `INITIAL_WORKSPACE_SHA256_FILE` | Optional path to a file containing the bare 64-character hexadecimal pin, with surrounding whitespace or one trailing newline allowed. A relative path resolves under the application root. |
| `CFP_ROUTING_POLICY_PATH` | Optional path to an operator-owned Arbiter routing policy. The built-in policy sends every valid category to generic program triage with no track assignment. A relative path resolves below the application root; the file must be a regular, non-symlink file no larger than 1 MiB. |
| `CFP_ROUTING_POLICY_SHA256` | Optional exact SHA-256 pin for the policy bytes. Use this or the file form, never both. Preview mode requires a pin whenever an external policy is configured. |
| `CFP_ROUTING_POLICY_SHA256_FILE` | Optional path to a file containing the policy pin. Rostrum reads and compiles the policy once before serving; changing it requires a deliberate restart. Category arguments are stable category IDs, not display labels, so policy cases must match the IDs shown in Settings or the exported workspace. |
| `STORE_DRIVER` | `json` (the default), `sqlite`, `postgres`, or `postgresql`. SQLite is a practical single-instance choice. |
| `DATA_PATH` | JSON or SQLite canonical-store path. It defaults under the application root to `data/rostrum.json` or `data/rostrum.sqlite` according to the driver. Use an absolute durable path in production. Postgres reads `DATABASE_URL` instead. |
| `DATABASE_URL` | Required only for `postgres`/`postgresql`; passed to the pgx driver and never echoed as the store path. |
| `AUDIT_LOG_PATH` | Independent fsynced, hash-chained JSON Lines ledger; defaults to `data/audit.log` under the application root. Keep it durable and separate from mutable workspace state. |
| `BACKUP_DIR` | Destination for the exact pre-import workspace backup; defaults to `data/backups` under the application root. Rostrum retains the newest ten automatic import backups. |
| `UPLOAD_DIR` | Durable speaker-file directory; defaults to `data/uploads` under the application root. It holds private uploads and the original approved portraits served by the public gallery; keep it on the same protected recovery boundary as the canonical workspace. |

`INITIAL_WORKSPACE_SHA256` and `INITIAL_WORKSPACE_SHA256_FILE` are mutually
exclusive, and either form requires `INITIAL_WORKSPACE_PATH`. A live deployment
may use an unpinned template, although pinning is the safer repeatable
operation. `APP_MODE=preview` requires the template plus exactly one pin and
refuses to start when its exact bytes differ. It also scans the complete
canonical workspace and refuses any email-like value whose domain is not
`example.com`, `example.net`, `example.org`, or a subdomain of one of those
reserved domains. This preview-only guard covers values embedded in proposals,
notification recipients, and audit metadata; live workspaces can use real
operational addresses normally.

`RESET_SECRET` is a disposable-workspace control, not a recovery plan. With it
empty, production returns 404 for reset. If an operator deliberately sets it,
an authorized reset restores the startup initial workspace and attempts to
clear the stored upload files, logging any individual removal failure. Keep it
empty for a real event and use validated import or an infrastructure snapshot
for recovery.

### Preview presentation

| Variable | Preview meaning |
| --- | --- |
| `PREVIEW_LABEL` | Optional short label shown in the read-only preview banner; defaults to `Read-only preview`. It does not enable preview mode. |
| `PREVIEW_MESSAGE` | Optional explanatory preview copy; Rostrum supplies generic read-only guidance when it is empty. It does not relax or replace the server-side controls. |

### Organizer identity

| Variable | Production meaning |
| --- | --- |
| `ORGANIZER_EMAILS` | Comma-separated organizer allowlist for magic-link and OAuth grants. Leave empty for first-host break-glass setup. |
| `PRINCIPAL_ROLES` | Strict `email=role+role,...` provisioning for organizer, chair, and observer access; `email=none` explicitly revokes access. A non-empty mapping must retain an organizer. |
| `AUTH_GITHUB_CLIENT_ID` / `AUTH_GITHUB_CLIENT_SECRET` | Set both to enable GitHub. Callback: `{PUBLIC_URL}/auth/oauth/github/callback`. |
| `AUTH_GITHUB_HANDLES` | Optional comma-separated, case-insensitive GitHub login handles allowed to bootstrap organizer access. This applies only to verified GitHub OAuth callbacks; durable email principals and `PRINCIPAL_ROLES` govern later access and revocation. |
| `AUTH_GOOGLE_CLIENT_ID` / `AUTH_GOOGLE_CLIENT_SECRET` | Set both to enable Google. Callback: `{PUBLIC_URL}/auth/oauth/google/callback`. Google email must be verified. |

### Email delivery

| Variable | Production meaning |
| --- | --- |
| `MAIL_DRIVER` | `outbox`, `smtp`, or `resend`. If unset, a Resend key wins, then SMTP host, then outbox. Set it explicitly in production. |
| `MAIL_FROM` | Envelope/header From value, for example `Rostrum <noreply@example.com>`. Required for a real transport to count as configured. |
| `RESEND_API_KEY` | Credential used by the Resend HTTP transport. |
| `RESEND_API_BASE_URL` | Defaults to the [Resend API](https://api.resend.com); override only for a compatible endpoint you control and test. |
| `SMTP_HOST` / `SMTP_PORT` | SMTP relay and submission port; port defaults to `587`. Use a STARTTLS-capable relay. |
| `SMTP_USER` / `SMTP_PASSWORD` | Optional SMTP PLAIN credentials. Leave both empty only for a relay that deliberately allows it. |

### Optional one-way publishing

| Variable | Production meaning |
| --- | --- |
| `ACCELEVENTS_EVENT_URL` / `ACCELEVENTS_API_KEY` | Unlock explicit Accelevents publishing. Use a restricted staging event first. |
| `ACCELEVENTS_BASE_URL` | Defaults to the Accelevents API; override only for a compatible test endpoint. |
| `AIRTABLE_PAT` / `AIRTABLE_BASE_ID` | Personal Access Token and target base for explicit Airtable projection. |
| `AIRTABLE_SPEAKERS_TABLE` / `AIRTABLE_SESSIONS_TABLE` | Target table names; default to `Speakers` and `Sessions`. |
| `AIRTABLE_API_BASE_URL` | Defaults to the [Airtable API](https://api.airtable.com/v0); override only for a compatible endpoint. |

Both integrations are one-way projections. Rostrum remains canonical. Their
credential-free dry runs are not evidence that a live provider accepted data;
follow the [deployment reference](deployment.md#accelevents-publishing) before
enabling either credential set.

## Storage, paths, and permissions

Rostrum persists four related but distinct classes of data:

| Data | Path or service | Recovery role |
| --- | --- | --- |
| Canonical workspace | `DATA_PATH` or `DATABASE_URL` | Event, CFP, submissions, reviews, speakers, schedule, identity, communications, and integration ledger |
| Uploads and approved public media | `UPLOAD_DIR` | Private speaker files and the original portrait files that may become public after organizer approval |
| Independent audit | `AUDIT_LOG_PATH` and its rotated segments | Tamper-evident operational history outside mutable workspace state |
| Pre-import backups | `BACKUP_DIR` | Automatic rollback point created before a validated workspace import |

`DATA_PATH` does not relocate uploads; `UPLOAD_DIR` does. The container keeps
both under `/app/data`, and the native example keeps both under
`/opt/rostrum/data`, so one protected volume captures the complete recovery
boundary. `UPLOAD_DIR` must be a dedicated directory rather than the
application root or filesystem root; a relative value resolves under the
application root. Do not point it at the directory that directly contains
`DATA_PATH`, `AUDIT_LOG_PATH`, or `BACKUP_DIR`: workspace reset is allowed to
clear files inside `UPLOAD_DIR`.

Never expose `UPLOAD_DIR` through a general-purpose static file server. Rostrum
keeps speaker files private through the authenticated `/portal-file/` route.
Only an active headshot task completion that an organizer has approved is
eligible at `/public-headshot/{speakerID}`. That public handler reopens the
original file inside `UPLOAD_DIR`, verifies it is a regular contained file and
an allowed image type, and serves nothing after approval is withdrawn. There
is no second public-media copy or second media volume to recover.

The application requests restrictive modes for sensitive files: JSON state,
the SQLite database and current sidecars, audit segments, backups, and uploads
are owner-only. Rostrum creates missing dedicated data directories privately,
but it never changes the mode of an existing arbitrary parent. The systemd
`UMask=0077` baseline, parent mount, backup destination, and container runtime
must prevent access by unrelated users. Never serve the data directory as
static content.

### JSON

JSON is the zero-configuration reference store. Each successful mutation
validates a cloned next state, writes and fsyncs a temporary file, then uses an
atomic rename. Operate one process and place the file on a filesystem where
atomic rename works within the same directory.

### SQLite

SQLite stores the validated workspace aggregate in one row, enables WAL mode,
uses one application connection, and sets a five-second busy timeout. Persist
the database file and its adjacent WAL/shm files by mounting the directory,
not a single file. Use one Rostrum process.

### Postgres

Set `STORE_DRIVER=postgres` and `DATABASE_URL` to an operator-owned database.
Startup connects, creates the compact workspace and migration tables when
needed, and loads or seeds the one canonical row. Database credentials remain
in the connection URL/environment. Rostrum passes that URL to the pgx driver,
so include the provider-required TLS and certificate parameters in it. Continue
to provide durable upload, audit-log, and backup paths; Postgres replaces only
the canonical workspace file.

## Reverse proxy and TLS

Rostrum listens over HTTP and expects a trusted edge to terminate TLS. Set
`PUBLIC_URL` to the browser-visible HTTPS origin even though the upstream hop
is HTTP. Keep the upstream private.

An Nginx-style baseline is:

Because this proxy connects over loopback, set
`TRUSTED_PROXY_CIDRS=127.0.0.1/32,::1/128` in the Rostrum service environment.
For a container or cluster, use only the exact private network from which the
proxy reaches the app.

```nginx
map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
}

server {
    listen 443 ssl http2;
    server_name program.example.com;

    client_max_body_size 34m;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_read_timeout 3600s;
    }
}
```

Adapt certificate directives and rate limits to your platform. The long read
timeout and upgrade headers preserve `/live` WebSocket refreshes. The 34 MiB
edge ceiling admits Rostrum's largest envelope: workspace import files are
capped at 32 MiB inside a 34 MiB multipart request. Rostrum still caps portal
upload envelopes at 12 MiB, individual uploads at 10 MiB, and ordinary request
bodies at 1 MiB. A proxy that supports route-specific limits can keep 34 MiB
only for `/organizer/import/workspace` and use 12 MiB elsewhere.

Rostrum ignores forwarding headers unless the direct network peer belongs to
`TRUSTED_PROXY_CIDRS`. It then walks `X-Forwarded-For` from right to left and
uses the closest untrusted address, so a visitor cannot choose a bucket by
prepending a value. This keeps submission, draft, preview, and magic-link
limits per visitor instead of sharing one proxy-wide bucket. Forwarding chains
are bounded, malformed trusted suffixes fall back safely, and invalid startup
configuration is rejected before listen. Keep edge rate limits too; the app
limits are defense in depth.

Do not add a blanket `X-Frame-Options` header and do not replace the
application CSP with one global policy. Rostrum allows framing only for
`/public/*` embeds and sends `frame-ancestors 'none'` elsewhere. A proxy-wide
deny breaks the documented embed feature; a proxy-wide allow weakens
organizer, reviewer, and speaker surfaces.

## Configure and test mail

### Resend

Set:

```dotenv
MAIL_DRIVER=resend
RESEND_API_KEY=<secret>
MAIL_FROM=Rostrum <noreply@your-domain.example>
```

The adapter sends plain text plus an optional `invite.ics` attachment and uses
an idempotency key. Keep the default API base URL unless you intentionally run
a compatible service.

### SMTP

Set:

```dotenv
MAIL_DRIVER=smtp
SMTP_HOST=smtp.example.com
SMTP_PORT=587
SMTP_USER=<username>
SMTP_PASSWORD=<secret>
MAIL_FROM=Rostrum <noreply@your-domain.example>
```

The implementation uses standard SMTP submission and upgrades with STARTTLS
when the relay advertises it. It does not document implicit-TLS port 465 as a
supported path. Use a relay that supports STARTTLS on the configured port and
test delivery to external recipients.

### Acceptance test

1. Complete organizer sign-in through the intended external URL.
2. Send a message from **Communications** to a controlled recipient.
3. Confirm receipt, From alignment, links, and any calendar attachment in the
   real mailbox.
4. Inspect the Communications ledger for sent, retrying, failed, and
   suppressed entries.
5. Restart Rostrum and confirm queued work resumes.

The persisted outbox runs once at startup and once per minute. Provider calls
are at-least-once: a process failure after delivery but before the completion
write can retry. Resend receives a stable idempotency key; SMTP cannot promise
the same provider-side de-duplication, so reconcile ambiguous retries against
the visible Communications ledger.

## Identity providers and passkeys

Magic links require only a tested mail transport and the organizer allowlist.
OAuth requires both variables for a provider; a half-configured pair does not
show a login button. Register these exact callbacks at the provider:

```text
https://program.example.com/auth/oauth/github/callback
https://program.example.com/auth/oauth/google/callback
```

Provider email is still checked against `ORGANIZER_EMAILS` or a stored
principal provisioned through `PRINCIPAL_ROLES`. GitHub may additionally use
`AUTH_GITHUB_HANDLES`; it still requires the provider's verified account email
for durable principal storage. Stored roles replace any provider claims; OAuth
is not a bypass around Rostrum authorization.

Passkeys need no separate environment credential. They use `PUBLIC_URL` as
the WebAuthn origin and can be registered only from an already authenticated
session. Test registration and a new-browser sign-in over the final hostname
and TLS certificate before relying on them.

Speaker and reviewer access is separate from organizer identity. Rostrum
issues scoped, signed links; do not put `/portal/*`, `/review/*`, or their
calendar/file routes behind an organizer-only proxy gate.

## Backup, export, and restore

Rostrum does not schedule off-host backups for you. Establish an operator-owned
schedule before collecting real submissions.

### What to collect

From **Settings → Export and restore**, an organizer or chair can download:

- A checksummed workspace JSON envelope. It excludes pending magic-link
  records and is the supported online state-import format.
- A full `.tar.gz` archive containing `workspace.json`, every regular file in
  `UPLOAD_DIR` (private files and approved portrait originals alike), and all
  active or rotated audit-ledger segments.
- A deterministic approved-file ZIP at
  `/organizer/export/approved-uploads.zip`. Its manifest identifies and hashes
  only approved task uploads; use it for an approved handoff, not as a complete
  backup.

Store full archives encrypted, off host, with retention appropriate for the
PII and reviewer material they contain. Listing an archive with `tar -tzf`
should show `workspace.json` plus any `uploads/` and `audit/` entries. Do not
extract an untrusted archive over a live application directory.

For infrastructure backups:

- Stop the process or use a storage/database-native consistent snapshot.
- For SQLite, snapshot the directory so the database, WAL, and shm files stay
  together.
- For Postgres, use the provider's tested backup mechanism in addition to the
  Rostrum full archive.
- Include the complete `UPLOAD_DIR` and all `AUDIT_LOG_PATH` segments.

### Restore the workspace

1. Start a compatible Rostrum version with its own working organizer identity.
2. Open **Settings → Export and restore** and upload the exported
   `workspace.json` envelope.
3. Rostrum verifies export version, schema version, checksum, and domain
   invariants before changing state.
4. Rostrum writes an exact pre-import backup to `BACKUP_DIR`, retaining the
   newest ten, and then replaces the workspace.
5. The receiving host keeps its current organizer principals, passkeys, and
   pending magic links. Imported upload references are rebased to its local
   `UPLOAD_DIR`.

### Restore uploaded files from a full archive

Full-archive file recovery is a stopped-process operation in this release:

1. Extract the archive into a new private staging directory.
2. Restore its `workspace.json` through the validated Settings flow first.
3. Stop Rostrum.
4. Copy the staged regular files from `uploads/` into the receiving configured
   `UPLOAD_DIR`, preserving owner-only access. This restores the originals for
   approved public portraits as well as private portal files.
5. Keep the staged `audit/` directory with the recovery record. Do not splice
   it into the receiving host's live audit chain.
6. Restart and verify several authorized portal downloads byte-for-byte.

The independent audit ledger belongs to the receiving instance. A workspace
restore intentionally does not rewrite it.

## Upgrades and rollback

Treat every upgrade as a state migration even when the release advertises no
manual migration command.

1. Check out the exact candidate and record its full SHA.
2. Run `make check` and `make build` with the pinned toolchain.
3. Review release notes and schema changes.
4. Download a full Rostrum archive and take a consistent database/volume
   snapshot.
5. Build and tag the image or native bundle with that immutable SHA.
6. Stop the old process. Never run two JSON/SQLite processes during a rolling
   replacement.
7. Start the candidate against the existing persistent boundary.
8. Watch startup logs. Rostrum validates loaded state; SQL backends apply their
   internal store migrations during open and have no separate migration CLI.
9. Run the acceptance checks below and one authenticated organizer workflow.

For rollback, stop the candidate first. Restore both the prior binary/image
and its pre-upgrade database/volume snapshot when the candidate may have
changed stored schema or data. Do not assume an older binary can read state
already written by a newer one. Preserve the failed candidate's logs and
archive for diagnosis.

## Monitoring and operational signals

Poll:

```bash
curl -fsS https://program.example.com/api/health
```

A healthy response contains `ok: true`, app name `Rostrum`, the configured
immutable version, and a current UTC timestamp. This endpoint proves the HTTP
process is serving and identifies the release. It is not a deep, recurring
database, mail-provider, disk, or audit-chain probe; startup performs the store
open/validation and audit-chain verification.

Also monitor:

- Process restarts and non-zero exits.
- Standard output/error for startup, communications, export, import, calendar,
  and upload errors.
- Free space and inode use on the complete data volume. Audit segments rotate
  at roughly 10 MiB but are retained rather than automatically deleted.
- Database availability and backup freshness with provider-native tooling.
- **Communications** counts for queued, retrying, failed, and suppressed mail.
- TLS expiry, external latency, and 4xx/5xx rates at the reverse proxy.
- `ROSTRUM_VERSION` drift between the intended and served release.

There is currently no separate readiness endpoint or Prometheus metrics
endpoint. Use the reverse proxy for access logs; Rostrum's application log is
focused on lifecycle and operational failures rather than one line per
request.

## Security hardening checklist

- Run `APP_ENV=production`, `APP_MODE=live`, and an immutable
  `ROSTRUM_VERSION`.
- Leave the build-only `GOSX_STATIC_EXPORT` variable unset in every runtime.
- Keep the application port private and serve one exact HTTPS origin.
- Store session, mail, OAuth, database, and integration credentials in a
  secret manager; never in source, images, support tickets, or screenshots.
- Give the service account write access only to the durable data paths.
- Encrypt backups and restrict them as speaker/reviewer PII.
- Preserve Rostrum's route-aware CSP and add proxy rate limits without trusting
  arbitrary forwarded headers.
- Restrict outbound egress to DNS, the configured database, mail provider,
  OAuth providers, and explicitly enabled publishing APIs.
- Leave Accelevents and Airtable credentials unset until staging dry runs and
  live acceptance succeed.
- Keep `/setup` tokens, signed portal/reviewer links, session cookies, and
  archive downloads out of logs and collaboration tools.
- Test recovery, not only backup creation.
- Follow the private process in [SECURITY.md](https://github.com/M31-Labs/rostrum/blob/main/.github/SECURITY.md) for vulnerabilities; do not post exploit details or real data publicly.

Rostrum caps ordinary bodies at 1 MiB, portal upload envelopes at 12 MiB,
individual uploaded files at 10 MiB, and workspace imports at 32 MiB. Allowed
portal files are PDF, PowerPoint, Keynote, PNG, JPEG, or WebP; headshots are
further restricted by extension and detected image bytes. These application
checks complement, rather than replace, proxy limits and malware-handling
policy.

## Production acceptance

Run the anonymous edge checks first:

```bash
curl -fsS https://program.example.com/api/health
curl -fsS https://program.example.com/api/v1/workspace
curl -sS -o /dev/null -w '%{http_code}\n' https://program.example.com/organizer
curl -sS -o /dev/null -w '%{http_code}\n' https://program.example.com/organizer/export/workspace.json
```

Expect valid JSON from the first two calls, a redirect to login from anonymous
`/organizer`, and `403` from the unauthenticated export. Then use a fresh
browser to verify:

1. Organizer sign-in by every method you intend to support.
2. Event identity, timezone, dates, tracks, rooms, and CFP copy before sharing
   a public URL.
3. One controlled submission, confirmation delivery, portal link, and file
   upload. Confirm the file is not public, then approve a headshot and verify
   only that portrait appears through `/public-headshot/{speakerID}`.
4. One reviewer assignment and signed reviewer link.
5. One schedule move, conflict block, publish action, public agenda, calendar,
   and JSON projection.
6. A workspace export, full archive, off-host copy, and staged restore drill.
7. A process restart with state, uploads, identity, and pending communications
   intact.

`make judge-demo` launches the isolated fictional observer example, while
`make smoke` verifies its local contract. Neither command is a production
organizer smoke test, and that contract is expected to reject live organizer
assumptions. Use the authenticated checklist above for the real host.

## Demo and production are separate artifacts

The public M31 evaluation deployment is not a starter database for a real
event. Its fictional event fixture, synthetic portraits, observer deployment
example, and judge-oriented verification belong under
[`examples/demo/README.md`](https://github.com/M31-Labs/rostrum/blob/main/examples/demo/README.md).

The example preparer writes a workspace template, its bare checksum file,
approved portrait originals, and an upload-checksum manifest into a disposable
or example-owned boundary. The generated workspace and checksum are
environment-specific runtime artifacts because stored upload paths are
absolute; none of the generated files belongs in the core module or a live
production image. The example Kubernetes init container accepts only a fresh
volume, then verifies the pinned workspace and the exact regular, non-symlink
portrait set and bytes on every restart.

That example may keep navigation, filters, persona links, and other safe
inspection interactions active while presenting the organizer workspace as an
anonymous observer: controls that create, move, publish, upload, export, or
save are not rendered, and server/store boundaries still refuse mutation.
Visitors are never organizers.

Production installation does not copy the example fixture. Use
`APP_MODE=live`, `INITIAL_WORKSPACE=fresh` or `INITIAL_WORKSPACE=empty`, a new
data boundary, a unique session secret, and operator-owned identity/provider
credentials. Never reuse the evaluation host, its volume, its fixture, or its
secrets for a real event.

## Troubleshooting

| Symptom | Check |
| --- | --- |
| Startup refuses the session secret | `SESSION_SECRET` must be unique and at least 32 characters for production or any non-local `PUBLIC_URL`. |
| Startup says production requires HTTPS | Set `PUBLIC_URL` to the final browser-visible HTTPS origin, even when the private upstream is HTTP. |
| `unsupported STORE_DRIVER` | Use `json`, `sqlite`, `postgres`, or `postgresql`. Postgres also requires `DATABASE_URL`. |
| No one-time setup URL appears | A stored organizer or non-empty `ORGANIZER_EMAILS` deliberately prevents break-glass setup. Do not delete state to force a new token. |
| Magic-link UI says sent but no mail arrives | `outbox` is non-networking. Verify `MAIL_DRIVER`, `MAIL_FROM`, and the selected SMTP/Resend credentials with an external mailbox. |
| OAuth button is absent | Both client ID and secret must be set for that provider before process start. Verify the exact callback derived from `PUBLIC_URL`. |
| Passkey origin error | Use the final HTTPS hostname and ensure `PUBLIC_URL` matches it exactly. Re-test from a supported browser after TLS is valid. |
| Upload returns 413 | Check both proxy limits and Rostrum's 12 MiB envelope/10 MiB file limits. Workspace import needs a separately scoped 32 MiB proxy allowance. |
| Upload succeeds but disappears after deployment | Persist `UPLOAD_DIR`; mounting `DATA_PATH` alone is not sufficient. |
| Preview rejects the initial workspace checksum | Configure exactly one of `INITIAL_WORKSPACE_SHA256` or `INITIAL_WORKSPACE_SHA256_FILE` and hash the exact raw template bytes, including whitespace. |
| Preview rejects a non-reserved email address | Do not use a live export. Replace every email-like value throughout the fictional template with an `example.com`, `example.net`, or `example.org` address (subdomains are allowed), regenerate the exact-byte checksum, and start from a new preview boundary. |
| SQLite is busy or loses state | Run one process and persist the directory containing the database plus WAL/shm files. |
| Initial-workspace change has no effect | Initialization is first-create only. Preserve the existing workspace; use the validated export/import workflow rather than replacing it casually. |
| Native bundle serves missing assets | Install the complete documented bundle paths and set `GOSX_APP_ROOT` to their common root. |
| Public embed is blocked | Remove proxy-wide `X-Frame-Options` or CSP replacement and preserve Rostrum's route-specific `frame-ancestors`. |
| Audit verification prevents startup | Preserve the ledger and investigate or restore a verified complete chain. Do not truncate it to make startup pass. |
| Older binary rejects the workspace schema | Stop it and restore the matching pre-upgrade binary plus state snapshot; do not edit schema numbers by hand. |

If a first boot created the wrong initial workspace and no real work exists,
stop Rostrum and move the new data boundary aside for inspection before
creating another one. Never remove or overwrite an ambiguous production store
as a troubleshooting shortcut.

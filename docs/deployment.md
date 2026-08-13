---
description: Advanced deployment, identity, persistence, integration, and observer-reference details for Rostrum operators.
nav_order: "06 / 08"
eyebrow: Extend one durable workspace
---

# Deployment reference

<!-- markdownlint-disable MD013 MD049 -->

Start with the [self-hosting manual](self-hosting.md) for the complete install,
first-organizer, proxy, mail, backup, upgrade, monitoring, security, and
acceptance path. This reference adds advanced deployment details, provider
projections, and the isolated observer example without repeating that runbook.

Rostrum deploys as one Go process plus the `app/` templates and `public/`
assets. One replica serves a single-organization instance.

For evaluation rather than operations, start with the
[judge and organizer guide](judging-guide.md). This document assumes an
operator is preparing a persistent live or read-only deployment.

## Build the production bundle

Requirements:

- The complete pinned toolchain in the
  [self-hosting prerequisites](self-hosting.md#prerequisites).

```bash
make check
make build
```

`make build` writes the `dist/` bundle: the static server binary at
`dist/server/app`, the route templates, framework runtime assets, public
files, and build metadata. The packaging step removes sourcemaps and Go source
that production does not need. The Dockerfile deliberately excludes
build-time `dist/data`; never treat prerender state as a runtime seed. Mount
one protected writable boundary for `DATA_PATH`, `UPLOAD_DIR`,
`AUDIT_LOG_PATH`, and `BACKUP_DIR`.

## Container

```bash
make build
docker build --build-arg ROSTRUM_VERSION="$(git rev-parse HEAD)" -t rostrum:local .
docker run --rm -p 8080:8080 \
  -e APP_MODE=live \
  -e APP_ENV=production \
  -e PUBLIC_URL=https://program.example.com \
  -e SESSION_SECRET='REPLACE_ME' \
  -e INITIAL_WORKSPACE=fresh \
  -e DATA_PATH=/app/data/rostrum.json \
  -e UPLOAD_DIR=/app/data/uploads \
  -e AUDIT_LOG_PATH=/app/data/audit.log \
  -e BACKUP_DIR=/app/data/backups \
  -v rostrum-data:/app/data \
  rostrum:local
```

The Dockerfile copies the verified `dist/` bundle. The image contains no Go
source and runs as the non-root `rostrum` user (UID 10001). In production
the process refuses to start with:

- the development session secret, or any secret shorter than 32 characters;
- a `PUBLIC_URL` that is not HTTPS;
- an in-memory `DATA_PATH`.

Set `ROSTRUM_VERSION` to the immutable release tag or commit SHA. The value is
returned by `GET /api/health`, so a deployment record can prove which Rostrum
build is serving. Check every live deployment directly:

```bash
curl -fsS https://program.example.com/api/health
```

Perform the authenticated live organizer workflow in the launch checklist
separately. The observer smoke contract under `examples/demo/` is expected to
fail against an organizer-gated live deployment.

## Kubernetes baseline

The reusable live baseline is
[`deploy/k8s/rostrum.yaml`](https://github.com/M31-Labs/rostrum/blob/main/deploy/k8s/rostrum.yaml),
with a deliberately placeholder-only starting point in
[`deploy/k8s/secret.example.yaml`](https://github.com/M31-Labs/rostrum/blob/main/deploy/k8s/secret.example.yaml).
It creates a dedicated namespace, one `ReadWriteOnce` persistent-volume claim,
a one-replica `Recreate` deployment, a service, and a TLS ingress. The pod runs
as UID/GID `10001`, drops Linux capabilities, and mounts the complete
`/app/data` recovery boundary.

Replace its six placeholders: the Rostrum image digest, matching immutable
`ROSTRUM_VERSION`, hostname, exact trusted-proxy CIDRs, Traefik ingress class,
and certificate issuer.
Resolve the published image to a registry digest and put that digest—not a
mutable tag—into `__ROSTRUM_IMAGE_BY_DIGEST__`. Create the real Secret out of
band and keep `RESET_SECRET` empty for a production workspace. Add only the
mail and identity credentials you actually accept. The manifest selects
`APP_MODE=live` and `INITIAL_WORKSPACE=fresh`; the normal path defaults keep
the audit ledger, backups, and uploads under the mounted volume. Review
storage class, resource sizing, network policy, TLS issuance, backup
integration, and log collection for your cluster before ingress is opened.
The included Traefik CRD middleware sets
`maxRequestBodyBytes: 35651584` (34 MiB) and
`memRequestBodyBytes: 2097152` (2 MiB), and the Ingress references it as
`rostrum-request-body-limit@kubernetescrd`. This requires Traefik's Kubernetes
CRD provider. If your cluster uses a different ingress controller, replace
both the middleware and annotation with its equivalent 34 MiB ceiling so the
application's tighter upload and import limits can run.

## Hosted observer preview

Preview mode is a generic product capability, not a fictional fixture baked
into the Rostrum module. An operator supplies a raw, identity-free workspace
JSON file and pins its exact bytes. The public M31 evaluation fixture,
synthetic media, launcher, smoke contract, and observer deployment live
separately under
[`examples/demo/README.md`](https://github.com/M31-Labs/rostrum/blob/main/examples/demo/README.md).

A preview uses a separate process, subdomain, volume, audit path, release
configuration, and session secret from every live organizer deployment:

```text
APP_MODE=preview
APP_ENV=production
PUBLIC_URL=https://preview.example.com
ROSTRUM_VERSION=<immutable-release-or-commit>
SESSION_SECRET=REPLACE_ME
INITIAL_WORKSPACE_PATH=/app/example/workspace.json
INITIAL_WORKSPACE_SHA256=<64-character-sha256-of-exact-file-bytes>
CFP_ROUTING_POLICY_PATH=/app/example/cfp-routing.arb
CFP_ROUTING_POLICY_SHA256=<64-character-sha256-of-exact-policy-bytes>
STORE_DRIVER=sqlite
DATA_PATH=/app/preview-data/rostrum.sqlite
UPLOAD_DIR=/app/preview-data/uploads
AUDIT_LOG_PATH=/app/preview-data/audit.log
BACKUP_DIR=/app/preview-data/backups
MAIL_DRIVER=outbox
ORGANIZER_EMAILS=
RESET_SECRET=
TRUSTED_PROXY_CIDRS=<exact-private-proxy-cidrs>
PREVIEW_LABEL=Evaluation workspace
PREVIEW_MESSAGE=Explore this read-only workspace without saving changes.
```

`APP_MODE=preview` fails closed unless the template path resolves absolutely,
exactly one of `INITIAL_WORKSPACE_SHA256` or
`INITIAL_WORKSPACE_SHA256_FILE` supplies a matching pin, storage is durable
JSON or SQLite, the release identity is immutable, organizer identity is
absent, and all database, network-mail, OAuth, Accelevents, and Airtable
credentials are absent. A checksum file contains the bare 64-character
hexadecimal digest, with surrounding whitespace allowed. Startup also compares
the complete persisted state with the decoded template; changed workspace or
identity state refuses to serve. Every email-like value anywhere in that state
must use `example.com`, `example.net`, `example.org`, or one of their
subdomains; any other email domain refuses startup. This address restriction
applies only to preview mode. The label and message change presentation only;
they do not enable preview mode or weaken its enforcement. The build-only
`GOSX_STATIC_EXPORT` variable must also be absent; preview startup rejects it.

The template is a raw `domain.State` JSON document, not the checksummed
operator export envelope accepted by Settings restore. Follow the example's
generation and checksum instructions rather than editing an exported live
workspace into place. Never use a template containing real event data. The
reserved-domain check is a last fail-closed barrier, not a data-anonymization
tool.

A preview keeps safe browser interaction—navigation, filters, disclosures,
persona inspection, and public itinerary behavior—while presenting
`/organizer/*` anonymously as an observer. Mutation forms and controls are not
rendered. Every unsafe method, authentication/setup route, reset, import,
export, and upload is refused, and a store-level wrapper remains read-only if a
future route misses the HTTP gate. Responses carry
`X-Robots-Tag: noindex, nofollow, noarchive`.

Follow the example README and deployment assets for the M31 evaluation host.
Its namespace, volume, generated workspace and upload checksums, public media,
and secret must remain separate from live. Its example-only init container
accepts only a fresh volume, then verifies the pinned workspace plus the exact
regular, non-symlink portrait set and bytes on every restart. `make judge-demo`
prepares, verifies, and launches the disposable local example; `make smoke`
verifies its local contract. Before sharing any remote preview, run the checks in
[launch readiness](launch-readiness.md#hosted-preview-acceptance) and the
example's smoke contract against the exact immutable version documented in
that README.

## Reverse proxy

Terminate TLS (Transport Layer Security) at a trusted reverse proxy and
preserve WebSocket upgrades for `/live`. Rostrum permits other sites to
frame its `/public/*` pages, because that is how the embeddable agenda
works; every other route sends `frame-ancestors 'none'`. Do not add a
blanket `X-Frame-Options` header. If your proxy replaces the application
CSP (Content Security Policy), keep the policy route-aware.

Set `TRUSTED_PROXY_CIDRS` to the exact loopback, container, or pod networks
from which that proxy connects. Rostrum accepts `X-Forwarded-For` only from a
listed peer and walks the chain right-to-left to find the closest untrusted
client. Never use `0.0.0.0/0` or `::/0`: an overly broad value lets callers
forge the IP identity used by submission and sign-in rate limits. The K8s
manifest deliberately leaves a `__TRUSTED_PROXY_CIDRS__` placeholder because
pod/service CIDRs are cluster-specific.

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

### Chairs and observers

Use `PRINCIPAL_ROLES` when access needs to be narrower or explicitly
governed:

```dotenv
PRINCIPAL_ROLES=owner@example.com=organizer,chair@example.com=chair,observer@example.com=observer
```

The mapping is strict and applied atomically at startup. Listed principals get
exactly their configured roles; omitted stored principals remain. At least one
organizer must remain. Use `former@example.com=none` for an explicit durable
revocation that also overrides the legacy allowlist. Every authenticated
request reconciles organizer authority against current durable roles, so old
cookies, pending magic links, and passkeys cannot retain a revoked grant.
OAuth provider claims cannot elevate them. Use `chair` for governed chair
overrides and `observer` for a read-only organizer surface without exports.

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

`POST /workspace/reset` restores the configured initial workspace and always
requires an authenticated organizer or chair. If `RESET_SECRET` is set, send
it in the POST form field named `secret` as a second factor; never put it in a
URL. In production with no secret set, reset is disabled entirely. A reset also attempts to clear the stored upload
files under `UPLOAD_DIR`; individual removal failures are logged. Inspect the
result rather than treating reset as a routine production recovery mechanism.

## Storage and replicas

The JSON store validates a cloned next state, then replaces the data file
with an atomic rename. SQLite uses the same aggregate contract with WAL;
Postgres uses the configured `DATABASE_URL`. Run exactly one application
replica. Postgres replaces the canonical workspace file, but it does not make
the upload directory or independent audit ledger multi-replica safe by itself.

Keep `DATA_PATH`, `UPLOAD_DIR`, `AUDIT_LOG_PATH`, and `BACKUP_DIR` on durable
operator-owned storage. The independent audit ledger is intentionally outside
the mutable workspace state: a workspace restore never rewrites the active
operational history.

`UPLOAD_DIR` is the single protected source for private portal files and
approved gallery portraits. Do not publish it as a static directory. Rostrum
serves a portrait at `/public-headshot/{speakerID}` only while an active
headshot task has an approved completion, and revalidates the contained file
and image type on each request. The full archive therefore recovers both
private uploads and approved public portrait originals from the same path.
Keep it as a dedicated child directory; never make it the directory that also
contains the canonical store, audit log, or backup files.

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
  `UPLOAD_DIR`.
- **Full archive** is a streaming `.tar.gz` containing `workspace.json`, all
  regular files from `UPLOAD_DIR` (including approved portrait originals), and
  every active or rotated audit-log segment. It is the cold-storage and
  file-recovery artifact.
- **Approved-upload bundle** at
  `/organizer/export/approved-uploads.zip` is a deterministic ZIP with a
  checksummed manifest and only approved task files. It is an operational
  handoff, not a complete recovery artifact.

Full-archive file recovery is deliberately a stopped-process operation in
this release. Extract an archive only into a new, private staging directory;
first restore its `workspace.json` through the validated Settings flow, then
stop Rostrum and copy the staged `uploads/` files into the receiving
`UPLOAD_DIR`. Restart and verify portal downloads byte-for-byte.
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
- Admit a 34 MiB envelope on `/organizer/import/workspace`; imported JSON is
  capped at 32 MiB. Rostrum accepts portal uploads to 10 MiB inside a 12 MiB
  request envelope and caps ordinary request bodies at 1 MiB.
- Public form submissions are rate limited per session and per IP address.
  The limiters are in-memory and reset on restart.

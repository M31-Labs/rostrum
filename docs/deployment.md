# Deployment guide

Rostrum deploys as one Go process plus the `app/` templates and `public/`
assets. One replica against one JSON data file serves a single-organization
instance.

## Build the production bundle

Requirements:

- Go 1.26 or newer.
- The GoSX CLI: `go install m31labs.dev/gosx/cmd/gosx@v0.38.0`
- The Arbiter CLI (used by `make check`): `go install m31labs.dev/arbiter/cmd/arbiter@v1.9.0`

```bash
make build
```

`make build` writes the `dist/` bundle: the static server binary at
`dist/server/app`, the route templates, framework runtime assets, public
files, and seeded data. The packaging step removes sourcemaps and Go source
that production does not need. Mount a writable directory for `DATA_PATH`
and for uploaded files under `data/uploads`.

## Container

```bash
make build
docker build -t rostrum:local .
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
- `/review/{token}` requires a signed reviewer link on every request (this
  compatibility window is documented in
  `hypha://m31labs/programma/specs/identity-plane.md`).

### Organizer allowlist

Set `ORGANIZER_EMAILS` to a comma-separated list of addresses. A magic-link
or OAuth (Open Authorization) sign-in from one of these addresses is
granted the organizer role and recorded as a workspace Principal, so the
grant survives a restart even if you later change the allowlist.

### Break-glass bootstrap

A fresh self-host with `ORGANIZER_EMAILS` empty and no stored organizer logs
a one-time setup URL at startup:

```
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

Issued magic-link tokens and registered passkeys persist in the same JSON
workspace file as everything else, so they follow the single-replica rule
below: run exactly one application replica. A future multi-instance
deployment needs a shared store behind the same `MagicLinkStore` and
`WebAuthnStore` interfaces.

Keep these routes reachable by the public: `/`, `/login`, `/submit/*`,
`/public/*`, `/api/health`, `/api/v1/*`, `/portal/*`, and `/review/*`. The
last two must stay reachable because speakers and reviewers open their
signed links from an inbox.

`POST /demo/reset` restores the seeded workspace. Set `RESET_SECRET` to
require a matching secret. In production with no secret set, reset is
disabled entirely.

## Storage and replicas

The JSON store validates a cloned next state, then replaces the data file
with an atomic rename. Run exactly one application replica against one data
volume. Back up the volume, including `data/uploads`, and test restoration.

## Operational configuration

- Use a secret manager for the session, SMTP, and Accelevents credentials.
- Restrict outbound egress to the provider APIs you configure.
- Poll `GET /api/health`; it returns the application name, version, and
  timestamp.
- Keep the Accelevents sync ledger and review provenance as audit data.
- Set the proxy body-size limit to 12 MiB or more. Rostrum accepts uploads
  to 10 MiB inside a 12 MiB request envelope and caps all other request
  bodies at 1 MiB.
- Public form submissions are rate limited per session and per IP address.
  The limiters are in-memory and reset on restart.

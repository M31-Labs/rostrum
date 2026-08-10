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

## Access boundary

The application enforces identity on most private routes itself:

- `/portal/*`, `/calendar/*`, and `/portal-file/*` require a signed speaker
  link or a bound session.
- `/review/{token}` requires a signed reviewer link on every request.
- `/organizer/export/submissions.csv` returns 403 without an organizer
  session.

The organizer workspace is the exception: `/organizer/*` carries no
application login in this release. Gate it at an identity-aware proxy.
Whoever the proxy admits to `/organizer` is trusted as an organizer for the
rest of that session. Cloudflare Tunnel plus Access is one suitable
topology; Rostrum does not depend on Cloudflare.

Keep these routes reachable by the public: `/`, `/submit/*`, `/public/*`,
`/api/health`, `/api/v1/*`, `/portal/*`, and `/review/*`. The last two must
stay reachable because speakers and reviewers open their signed links from
an inbox.

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

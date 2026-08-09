# Deployment guide

Programma ships as one Go process plus the `app/` templates and `public/`
assets. The default JSON store is appropriate for a single replica,
self-hosted demonstration, or evaluation environment.

## Production bundle

```bash
make build GOSX=../gosx-programma-islands/bin/gosx
cd dist
./run.sh
```

The current module uses an adjacent `../gosx-programma-islands` development
worktree and a matching CLI; GoSX is not vendored. The build contains the
server binary, five binary island programs, only framework-owned browser
runtime assets, and the runtime content tree. The packaging step removes
generated sourcemaps and copied Go source that are not needed in production.
Mount a writable directory for `DATA_PATH` and uploaded files.

## Container

```bash
make build GOSX=../gosx-programma-islands/bin/gosx
docker build -t programma:local .
docker run --rm -p 8080:8080 \
  -e APP_ENV=production \
  -e PUBLIC_URL=https://program.example.com \
  -e SESSION_SECRET='replace-with-a-random-secret-of-at-least-32-characters' \
  -e DATA_PATH=/app/data/programma.json \
  -v programma-data:/app/data \
  programma:local
```

The Dockerfile consumes that verified `dist/` bundle rather than rebuilding
against a different framework revision. It copies the stripped server, `.gsx`
templates, hashed GoSX runtime/island assets, public files, and seeded data; it
contains no Go source and runs as the non-root `programma` user. The process
refuses to start in production with the development session secret, a
non-HTTPS public URL, or the in-memory store.

## Reverse proxy and identity boundary

Terminate TLS at a trusted reverse proxy and preserve WebSocket upgrades for
`/live`. The application deliberately permits its public agenda and speaker
gallery to be framed by other sites. Do not add a blanket `X-Frame-Options`
header; use route-aware `frame-ancestors` policy if your proxy replaces the
application CSP.

This submission does not include a production tenant identity plane. Before a
public deployment, protect `/organizer/*`, `/organizer/export/*`, `/portal/*`,
`/calendar/*`, `/portal-upload/*`, and `/demo/reset` with an identity-aware
proxy or application authentication. Keep `/`, `/submit/*`, `/public/*`,
`/api/health`, and the intentionally public `/api/v1/*` projections open as
needed. Cloudflare Tunnel plus Access is one suitable topology for a demo;
Programma itself does not depend on Cloudflare.

## Storage and replicas

The JSON store uses copy-on-write validation, file synchronization, and atomic
rename. Run exactly one application replica against a JSON data volume. A
multi-replica SaaS deployment should replace the `appstate` storage boundary
with tenant-scoped transactional storage and move uploads to object storage
with antivirus scanning and signed download URLs.

## Operational configuration

- Back up the data volume and test restoration.
- Use a secret manager for session, OpenAI, and Accelevents credentials.
- Restrict outbound egress to configured provider APIs.
- Poll `GET /api/health`; it returns runtime name, version, and timestamp.
- Preserve the Accelevents sync ledger and AI evaluation provenance as audit
  data.
- Put a body-size limit at the proxy no smaller than Programma's 12 MiB upload
  envelope; accepted files are limited to 10 MiB.

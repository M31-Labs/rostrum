# Rostrum fictional evaluation example

This directory is the complete boundary around the public Rostrum showcase.
It contains the fictional M31 Systems Forum workspace, synthetic portraits,
observer copy, exact route assertions, and the isolated deployment recipe.
The Rostrum server does not import this fixture and the base container does
not contain it. The example stays in the root Go module only so the normal
`go test ./...` gate validates it; an automated boundary test prevents any
non-test production package from importing it.

Use this example to evaluate the product—not as the starting point for a real
event. Organizers should follow the [self-hosting manual](../../docs/self-hosting.md),
which starts from Rostrum's small `fresh` workspace and creates a real first
organizer.

## Run it locally

From the repository root:

```sh
make judge-demo
```

The runner builds the current Rostrum source, prepares a validated workspace
and durable portrait uploads in a private temporary directory, pins the exact
workspace and portrait bytes with SHA-256, boots Rostrum in generic
`APP_MODE=preview`, and runs the full smoke contract. Start at
<http://127.0.0.1:8080/tour>. Press `Ctrl-C` to stop; the runner removes its
process and temporary files.

To verify the same contract without leaving the server running:

```sh
make smoke
```

To check an already deployed example, require the exact immutable version:

```sh
SMOKE_EXPECTED_VERSION=<release-or-commit> \
  examples/demo/smoke.sh https://your-preview.example
```

The remote smoke test proves the version, no-index posture, fixture counts,
public projections, observer-only organizer pages, signed example personas,
approved portraits, and mutation rejection before it sends representative
blocked-action probes.

The public CFP is the one deliberate presentation exception: it renders real
fields plus **Save draft** and **Submit proposal** buttons so a judge can try
the intake flow. Those actions are a client-only walkthrough that updates an
in-page status message; they never send a request, email, or workspace
mutation.

The organizer agenda has the same carefully bounded rehearsal treatment. A
judge can drag a card onto an open cell, see the card move locally, try an
occupied room to see the conflict explanation, and reset the board. The
agenda rehearsal is browser-only: it does not submit the move action, write
storage, or alter the deterministic fixture.

## What lives here

| Path | Purpose |
| --- | --- |
| `fixture/` | Fictional workspace builder and its own validity tests |
| `assets/headshots/` | Synthetic portraits used as ordinary approved uploads |
| `rules/` | Fictional category owners, queues, and tracks loaded only by this example |
| `prepare/` | Materializes environment-specific upload paths, workspace JSON, and checksums |
| `run.sh` | Disposable local preview runner |
| `smoke.sh` | Local and remote evaluation contract |
| `Dockerfile` | Optional example-only overlay on an immutable base Rostrum image |
| `k8s.yaml` | Isolated namespace, volume initialization, and read-only preview workload |

Generated `initial-workspace.json`, `initial-workspace.sha256`,
`uploads.sha256`, and `cfp-routing.sha256` files are deliberately not
committed. Approved upload paths
are absolute runtime paths, so `prepare` creates and hashes them for the
environment that will serve them.

## Build the hosted-example image

Build and verify the ordinary Rostrum bundle and base image first. Use one
immutable version value throughout:

```sh
revision="$(git rev-parse HEAD)"
make release-check
docker build --build-arg ROSTRUM_VERSION="$revision" -t "rostrum:$revision" .
docker build \
  -f examples/demo/Dockerfile \
  --build-arg ROSTRUM_IMAGE="rostrum:$revision" \
  -t "rostrum-example:$revision" .
```

Push the overlay under a unique tag, resolve its registry digest, and put the
digest plus the same `revision` into `k8s.yaml`. The init container copies the
example template and uploads into a new, empty preview volume, then verifies
the template and every portrait against image-owned SHA-256 pins on every
restart. Rostrum also verifies the workspace checksum and full persisted-state
fingerprint before serving anonymous reads. The manifest expects a Traefik
ingress class with the Kubernetes CRD provider enabled; its attached buffering
middleware accepts the app's multipart envelopes and rejects request bodies
above 34 MiB.

Never point this workload at a live namespace, database, upload directory,
audit ledger, session secret, or integration credential. Preview mode rejects
those configurations, and the example manifest uses a distinct namespace and
volume so a deployment error fails closed.

## Boundary contract

- Core defaults are `APP_MODE=live` and `INITIAL_WORKSPACE=fresh`.
- `APP_MODE=preview` is reusable observer infrastructure, not fixture logic.
- Core production files must never import `examples/demo`.
- Core embeds only a generic unassigned triage policy; the fictional routing
  policy is loaded from this directory with an exact SHA-256 pin.
- Portraits pass through the same durable upload, organizer-approval, and
  public-serving path used by a real installation.
- Demo-specific slugs, counts, people, prose, deployment values, and smoke
  assertions remain in this directory.

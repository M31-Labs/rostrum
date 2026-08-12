---
description: Evaluate, understand, and operate Rostrum from one judge-first field guide.
nav_order: "01 / 07"
eyebrow: Evaluate · understand · operate
home: true
mermaid: true
---

# Rostrum documentation

Rostrum keeps a complicated speaker program legible—from the first proposal
to the published room schedule. This documentation separates three different
questions: how to evaluate it, how it works, and what an operator must still
prove before a production launch.

![Rostrum's guided product tour follows five people through one canonical program record](images/product-tour.webp)

## Start here

| I am… | Read this | Outcome |
| --- | --- | --- |
| A judge or reviewer | [Judging guide](judging-guide.md) | Complete the five-minute tour or run the full interactive path |
| An event organizer | [README quickstart](https://github.com/M31-Labs/rostrum/blob/main/README.md#run-the-complete-product-locally) | Start a disposable seeded workspace and become its first organizer |
| A technical reviewer | [Architecture](architecture.md) | Understand components, trust boundaries, persistence, and limitations |
| An integration author | [API reference](api.md) | Consume the published event directory, schedule, and speakers |
| An operator | [Deployment guide](deployment.md) | Build a container, configure identity, storage, and a read-only preview |
| A release owner | [Launch readiness](launch-readiness.md) | Collect the evidence still required for a production decision |
| A contributor or designer | [Visual system](visual-system.md) | Preserve Rostrum's Paper & Ink editorial field guide |

## Evaluation paths

The local interactive run is the authoritative path for feature testing. It
uses the repository's seeded event and permits submissions, review, scheduling,
publishing, uploads, and reset. The hosted preview is a separate read-only
deployment and should be used only after its preflight passes.

```bash
# Authoritative interactive path
cp .env.example .env
DEMO_MODE=memory go run .

# Deterministic read-only judge path
make judge-demo

# Convenience preview preflight
curl -fsS https://rostrum.m31labs.dev/api/health
curl -fsS https://rostrum.m31labs.dev/api/v1/workspace
```

The terminal prints the local one-time `/setup?token=…` URL. Open that exact
URL to create the first organizer. The [judging guide](judging-guide.md)
explains the expected output and fallback behavior.

## Product map

```mermaid
flowchart LR
    CFP["Call for speakers"] --> Route["Governed routing"]
    Route --> Review["Human review"]
    Review --> Decision["Accept / decline"]
    Decision --> Portal["Speaker portal & tasks"]
    Decision --> Agenda["Conflict-aware agenda"]
    Portal --> Publish["Publish"]
    Agenda --> Publish
    Publish --> Public["Agenda · gallery · API · calendars · embeds"]
```

Every stage reads and writes the same validated workspace. Rostrum does not
maintain a second publishing database.

## Source landmarks

| Area | Location |
| --- | --- |
| File-routed pages and server actions | [`app/`](https://github.com/M31-Labs/rostrum/tree/main/app/) |
| Domain state and invariants | [`internal/domain/`](https://github.com/M31-Labs/rostrum/tree/main/internal/domain/) |
| JSON, SQLite, and Postgres stores | [`internal/store/`](https://github.com/M31-Labs/rostrum/tree/main/internal/store/) |
| Public serialization boundary | [`internal/publicapi/`](https://github.com/M31-Labs/rostrum/tree/main/internal/publicapi/) |
| Policy source and adapter | [`rules/`](https://github.com/M31-Labs/rostrum/tree/main/rules/) |
| Production manifests | [`deploy/k8s/`](https://github.com/M31-Labs/rostrum/tree/main/deploy/k8s/) |
| Rendered-flow smoke test | [`scripts/smoke.sh`](https://github.com/M31-Labs/rostrum/blob/main/scripts/smoke.sh) |
| Size and runtime budgets | [`size-budget.json`](https://github.com/M31-Labs/rostrum/blob/main/size-budget.json), [`perf-budget.json`](https://github.com/M31-Labs/rostrum/blob/main/perf-budget.json) |

## Honest status

The repository contains a complete credential-free local workflow and a
fail-closed configuration for a fictional read-only preview. Credential-backed
email, a chosen external database endpoint, live Accelevents/Airtable
publishing, backup recovery, and the exact hosted release still require
operator acceptance. No document in this directory treats a unit test or demo
outbox entry as proof of an external provider.

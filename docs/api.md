---
description: The small, cacheable, read-only public data contract for Rostrum events, schedules, speakers, and calendars.
nav_order: "04 / 07"
eyebrow: Consume published records
---

# Public API reference

<!-- markdownlint-disable MD013 -->

Rostrum exposes a deliberately small, read-only JSON API for published event
data. It is a projection, not a serialization of internal workspace state.

Base URL in a local run: `http://localhost:8080`.

## Conventions

- All `/api/health` and `/api/v1/*` endpoints use `GET` and return JSON with
  `Content-Type: application/json; charset=utf-8`.
- Timestamps are RFC 3339 values. Event/session timestamps retain their
  configured offset; the health timestamp is UTC.
- Public API responses use `Cache-Control: public` and an ETag.
- Unknown or unpublished material is omitted; there is no draft flag in the
  public projection.
- There is no authentication for `/api/health` or `/api/v1/*`.
- Rostrum does not currently publish an OpenAPI document.

## Health

`GET /api/health`

```bash
curl -fsS http://localhost:8080/api/health
```

```json
{
  "app": "Rostrum",
  "ok": true,
  "time": "2026-08-12T20:23:05Z",
  "version": "<deployment release or commit>"
}
```

Cache: 30 seconds. `version` comes from `ROSTRUM_VERSION`; operators should set
it to an immutable release tag or commit SHA.

## Workspace directory

`GET /api/v1/workspace`

This is the discovery endpoint. It returns public event metadata, published
counts, and stable links to the other projections and public pages.

```bash
curl -fsS http://localhost:8080/api/v1/workspace
```

```json
{
  "name": "Rostrum public API",
  "version": "v1",
  "event": {
    "id": "evt_m31_forum_2026",
    "name": "M31 Systems Forum 2026",
    "slug": "m31-systems-forum-2026",
    "theme": "Tools for governed, collaborative intelligence",
    "description": "…",
    "location": "San Francisco, California",
    "timeZone": "America/Los_Angeles",
    "startsAt": "2026-10-14T09:00:00-07:00",
    "endsAt": "2026-10-16T18:00:00-07:00"
  },
  "counts": {
    "publishedSessions": 6,
    "publishedSpeakers": 6
  },
  "links": {
    "self": "/api/v1/workspace",
    "schedule": "/api/v1/schedule",
    "speakers": "/api/v1/speakers",
    "agenda": "/public/m31-systems-forum-2026/agenda",
    "gallery": "/public/m31-systems-forum-2026/speakers"
  }
}
```

Counts above describe the deterministic demo seed. A fresh or operator-owned
workspace returns its own values. Cache: 60 seconds.

## Published schedule

`GET /api/v1/schedule`

```bash
curl -fsS http://localhost:8080/api/v1/schedule
```

Top-level fields:

| Field | Type | Meaning |
| --- | --- | --- |
| `eventId` | string | Canonical event ID |
| `timeZone` | string | IANA event timezone |
| `sessions` | array | Published sessions ordered by start time, then title |

Each session contains:

| Field | Type |
| --- | --- |
| `id`, `title`, `description`, `format` | string |
| `trackId`, `track`, `roomId`, `room` | string |
| `speakerIds` | array of strings |
| `startsAt`, `endsAt` | RFC 3339 timestamp |
| `status` | string; always `published` in this projection |

Draft, cancelled, and unscheduled sessions are not returned. Cache: 60
seconds.

## Published speakers

`GET /api/v1/speakers`

```bash
curl -fsS http://localhost:8080/api/v1/speakers
```

The response contains `eventId` and a name-sorted `speakers` array. A speaker
appears only when attached to at least one published session.

| Field | Type |
| --- | --- |
| `id`, `name`, `pronouns`, `role`, `company`, `biography`, `city` | string |
| `headshotUrl`, `websiteUrl`, `linkedInUrl` | string |
| `sessionIds` | array of published session IDs |

Email addresses, private task state, proposal text, and review data are never
part of this endpoint. Cache: 60 seconds.

## Public event calendar

`GET /public-calendar/{event-slug}.ics`

```bash
curl -fsS -o rostrum-program.ics \
  http://localhost:8080/public-calendar/m31-systems-forum-2026.ics
```

This anonymous RFC 5545 feed contains the same published sessions as the
public agenda. Stable session IDs become stable `VEVENT` UIDs. Draft,
cancelled, and unscheduled sessions are excluded, and no speaker identity or
private workspace state is added beyond already-public session content.

Response type: `text/calendar; charset=utf-8`. Cache: public, 60 seconds.

## Private downloads

These routes are interfaces but are not anonymous public API endpoints:

| Route | Authority | Response |
| --- | --- | --- |
| `GET /calendar/{speaker}.ics?key={signed-token}` | Matching signed token, bound speaker session, or organizer-facing session | RFC 5545 calendar containing published sessions for that speaker |
| `GET /portal-file/{completion}` | Owning speaker session or organizer-facing session | Private approved/uploaded file |
| `GET /organizer/export/submissions.csv` | Organizer or chair | Formula-safe submissions CSV |
| `GET /organizer/export/workspace.json` | Organizer or chair | Checksummed versioned workspace envelope |
| `GET /organizer/export/archive.tar.gz` | Organizer or chair | Workspace, uploads, and audit segments |
| `GET /organizer/export/approved-uploads.zip` | Organizer or chair | Deterministic approved-file bundle plus manifest |

Organizer downloads return `Cache-Control: private, no-store`. Unauthorized
private file and calendar requests avoid revealing whether the requested
record exists.

## Compatibility

The path prefix is the API version. Additive fields may appear within `v1`;
clients should ignore unknown object members. A breaking shape change should
use a new path prefix. The current repository does not promise a release
cadence or hosted availability SLA.

The serializer and its privacy contract are tested in
[`internal/publicapi/public_test.go`](https://github.com/M31-Labs/rostrum/blob/main/internal/publicapi/public_test.go).

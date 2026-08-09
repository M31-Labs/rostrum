# Programma submission narrative

## One sentence

Programma is a governed program-operations SaaS: one calm workspace takes an
event from conditional CFP through defensible review, speaker readiness,
conflict-free scheduling, and public distribution.

## The problem beneath the feature list

Speaker-program tools usually split one workflow across form builders,
spreadsheets, inbox reminders, calendar grids, file requests, and an external
event platform. The visible cost is busywork. The deeper cost is that routing,
review, and scheduling decisions become hard to explain once they cross those
boundaries.

Programma treats the event program as a governed system. The organizer remains
the owner; automation proposes or enforces only the policies the organizer can
see. Every downstream surface—the portal, public agenda, calendar feed, API,
and Accelevents export—derives from the same committed program state.

## Why GoSX and M31 belong in the story

- GoSX makes the server-rendered workflow the default and reserves client
  runtime for interaction that earns it.
- All browser behavior is expressed as route-scoped GoSX islands, Controllers,
  Hubs, Actions, navigation, or declarative disclosure: Programma ships zero
  bespoke JavaScript files and zero application JavaScript bytes.
- Arbiter turns changing conditional, routing, and conflict logic into named,
  testable policy instead of scattered `if` statements.
- GoSX Hubs make operational progress live without turning the entire product
  into a client-state application.
- The result demonstrates an M31 Labs thesis in a legible commercial product:
  powerful systems can remain inspectable, local-first, and structurally calm.

## Trust contract

1. Human review coverage and aggregate scoring include human evaluations only.
2. AI assistance is optional, rubric-bound, PII-minimized, schema-validated,
   provenance-labeled, and never changes submission status.
3. Schedule mutations run the same server conflict policy whether initiated by
   drag/drop or an accessible form.
4. External event publishing is one-way, explicit, credential-gated, and
   written to a visible ledger.
5. Public APIs are projections, not serialized workspace state.

## Judge-ready proof points

- Submit a workshop and watch the conditional question and category route fire.
- Edit the newly created speaker profile after the redirect; the organizer
  workspace receives the live update.
- Add one human review and one AI assist; only the human review advances
  coverage and the aggregate.
- Attempt a hard agenda collision and see the governing rule reject it.
- Save a personal itinerary at mobile width and reload it.
- Inspect the production manifest: the public root is navigation-only, while
  organizer and public-agenda routes load only their exact declared GoSX
  capabilities and compact binary island programs.
- Inspect public JSON and verify the absence of emails, review content, upload
  paths, proposal bodies, and drafts.
- Run an Accelevents dry run without credentials; configure a key to reveal the
  separate live-publish action.

## Sensible next SaaS layer

The submission deliberately proves the vertical workflow before adding a
horizontal control plane. The next layer is tenant identity and entitlements,
Postgres-backed multi-instance transactions, object storage and scanning,
durable job delivery, audit retention, billing, and organization-level data
residency. Those concerns can sit around the same domain, action, policy, and
projection boundaries without rewriting the product experience.

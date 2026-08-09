# Programma Product Specification

Programma is a GoSX-native, open-source speaker and program operations system.
It replaces the subset of Sessionboard needed to run a call for speakers, select
content, onboard speakers, build a conflict-free agenda, and publish the result.

## Product intent

- Give organizers one fast operating surface from CFP launch through published agenda.
- Give speakers a self-service portal that removes biography, file, and deadline work from email threads.
- Keep workflow decisions explainable and editable through Arbiter rule files.
- Remain useful without paid infrastructure: one Go binary, one data file, and optional external adapters.
- Offer a coherent API and integration boundary instead of making the UI the only way to move data.

## Source brief

The build targets the nine primary capabilities in the hackathon brief and the
annotated screenshots in the linked Google document. Product behavior was also
cross-checked against the official Sessionboard help center and the official
Accelevents API documentation.

- Brief: https://docs.google.com/document/d/1rBHJtiNKHv4i43tdf2Rm0sDEYuIcajhmAPoBKR_Az-A/edit
- Sessionboard overview: https://learn.sessionboard.com/get-started/overview
- Accelevents API: https://developer.accelevents.com/docs/accelevents-api-documentation

## Users and jobs

### Program organizer

Configure the event, launch forms, triage submissions, monitor review and
onboarding progress, communicate with speakers, schedule accepted sessions,
resolve conflicts, and publish the program.

### Reviewer

Work through assigned submissions without seeing other reviewers' scores,
apply a weighted rubric, leave internal comments, and optionally request an AI
second opinion.

### Speaker or submitter

Submit a proposal, land directly in the portal, maintain profile information,
complete required tasks and forms, upload files, read event resources, and add
confirmed sessions to a personal calendar.

### Website visitor

Browse a mobile-friendly agenda or speaker gallery, search and filter the
program, and download an iCalendar feed without signing in.

## Required capability matrix

| # | Brief requirement | Programma behavior | Demo evidence |
|---|---|---|---|
| 1 | Custom CFP forms with conditional logic and category routing | Schema-driven field catalog, show/hide rules, close date, category ownership, and `.arb` routing rules | Inspect the seeded CFP, preview the workshop-only field, submit into its routed queue |
| 2 | Self-service speaker portal for bios, headshots, slides, and documents | Profile editor, task list, portal forms, file requests, upload metadata, and session summary | Submit a proposal, arrive in the portal, update a biography, complete one task |
| 3 | Automated templated communication, reminders, and calendar invites | Merge-tag templates, scheduled reminder ledger, Gmail/Outlook handoff, demo outbox, and standards-compliant `.ics` attachments/feeds | Preview a rendered acceptance email and download a speaker-specific calendar invite |
| 4 | Multi-round scoring with optional AI assistance | Review plans, weighted rubrics, reviewer assignments, anonymized mode, round state, and optional OpenAI rubric assist | Score a proposal in round two and compare the human aggregate with the AI assist |
| 5 | Drag/drop agenda with automatic conflicts and list/day/week/track/room views | Progressive drag/drop schedule board, server schedule action, views, and room/track/speaker conflict policy | Move a session into a collision, see the conflict, then move it to a safe slot |
| 6 | Real-time outstanding onboarding dashboard | GoSX Hub broadcasts task updates; dashboard and speaker roster subscribe to aggregate progress | Complete a portal task in one tab and see organizer counts update in another |
| 7 | Native one-way Accelevents integration | Dry-run payload preview plus authenticated speaker/session upsert adapter and sync log | Preview mapped payloads, then run a credential-gated one-way sync |
| 8 | Portal resource/wiki pages with HTML embeds | Ordered resources support article, link, download, and allowlisted embed blocks | Open the speaker guide and embedded reference resource |
| 9 | Embeddable mobile speaker gallery and itinerary | Standalone responsive agenda, itinerary, session list, speaker list/gallery, JSON, and iCalendar endpoints | Open embed preview at mobile width and copy the generated embed snippet |

## Annotated-brief details

The screenshots add several acceptance details that are easy to miss in the prose:

- Event setup includes name, slug, type, website, location, timezone, dates, theme, and artwork.
- A submission form distinguishes abstract, session, and participant information.
- Title, first name, last name, and email are locked required fields.
- Submission payments are explicitly out of scope for this build.
- Form close date matters.
- Successful submission must redirect into the speaker portal.
- Submission confirmation is required; admin/update alerts are secondary.
- Speakers must be able to update their own biography and links.
- Organizer tables need status editing, filters, configurable columns, and import/export affordances.
- Portal work includes manual tasks, forms, and file requests.
- The dashboard should expose submission pipeline, review progress, speaker tracking, and scheduled-session health.

## Information architecture

### Organizer

- Today: event pulse, submission pipeline, review progress, onboarding health, and conflicts.
- Forms: CFP list, builder, preview, share link, routing policy, and notifications.
- Submissions: searchable table, status lanes, bulk actions, detail drawer, and export.
- Review: plans, rounds, rubric progress, evaluator queue, aggregate results, and AI assist.
- Speakers: profile completeness, outstanding work, sessions, files, and communication history.
- Agenda: unscheduled bank, calendar board, view controls, conflicts, and session editor.
- Communications: templates, recipient segments, scheduled reminders, previews, and outbox.
- Portal: task/form/resource configuration and participant preview.
- Embeds: gallery/itinerary/list configuration, code snippets, JSON, and iCalendar feeds.
- Integrations: Accelevents mapping, dry run, credentials state, and sync history.
- Settings: event details, taxonomy, branding, data path, and demo controls.

### Speaker portal

- Home: welcome, acceptance state, completion meter, next deadline, and session details.
- Tasks: manual work, forms, file requests, due dates, status, and completion actions.
- Profile: biography, headshot URL, company, role, pronouns, and public links.
- Files: uploaded slides and supporting-document records.
- Resources: articles, links, downloads, and allowlisted embeds.

### Public and embeddable

- CFP submission form.
- Schedule itinerary.
- Agenda grid.
- Session list.
- Speaker gallery and speaker list.
- JSON and iCalendar feeds.

## Execution architecture

Programma keeps the GoSX primitives separate:

- Server components render navigation, tables, forms, portal pages, and public embeds.
- Named Actions own browser mutations, validation, CSRF, and redirect-safe state.
- The GoSX navigation runtime upgrades internal links and managed forms to same-document transitions while complete server-rendered routes remain the fallback.
- Five focused GoSX islands compile to binary VM programs: workspace keyboard/search state, agenda drag/drop, clipboard feedback, conditional CFP fields, and the public itinerary. No application-authored JavaScript is shipped.
- GoSX declarative disclosure owns the review-method dialog, including Escape, focus trap, backdrop close, and trigger restoration, without an island.
- Route-scoped Controllers persist the organizer rail and public itinerary directly into typed shared signals.
- GoSX Hub bindings force a debounced, scroll-preserving soft refresh only on the organizer overview and portal matrix routes after durable task or schedule changes.
- Managed ActionForms project server messages and field errors into form-local live regions while preserving native POST/redirect/get fallbacks.
- Arbiter rules decide conditional field visibility, category routing, and schedule-conflict severity.
- Ordinary JSON API routes serve integrations, embeds, and non-form clients.

## Persistence and deployment

- Default: atomic JSON-file store with seeded demo data and no external service requirement.
- Scaling path: replace the single-process store boundary with tenant-scoped transactional storage.
- Deployment unit: one Go binary plus public assets and a writable data volume.
- Demo mode never requires secret credentials. External sends and syncs become previews or outbox records.
- Production mode requires an explicit session secret and HTTPS public URL; OpenAI and Accelevents remain optional.

## Safety and trust boundaries

- The server validates all mutations; client checks are feedback only.
- Browser form posts use GoSX session and CSRF protection.
- The submission has no production tenant identity plane; organizer and portal routes must be gated before public deployment.
- Uploaded file names and content types are recorded defensively; production storage is adapter-driven.
- Resource HTML uses a small allowlist and never renders arbitrary untrusted markup.
- AI review receives proposal content and rubric context, not speaker email or private onboarding data.
- Accelevents sync is one-way, uses stable external IDs, stops on remote errors, and writes a visible run ledger.
- Every routed or conflict decision can expose a human-readable rule trace.

## Visual System

### Territory

Swiss Precision. The interface uses a strict modular grid, flush-left type,
visible rules, narrow status rails, and oversized section numerals. The
signature move is a thin vermilion signal line that connects a page title to
its live operational state. Decoration never competes with dense planning work.

### Typography

- Display: Space Grotesk, weight 600.
- Body: Work Sans, weights 400, 500, and 600.
- Mono: IBM Plex Mono, weight 500, for identifiers, time, and machine state.
- Scale: minor third (1.2), tuned with fluid clamps for compact product UI.
- Line height: 1.1 for display, 1.35 for controls, and 1.55 for prose.

Type steps:

- `--type-xs`: `clamp(0.72rem, 0.69rem + 0.12vw, 0.78rem)`
- `--type-sm`: `clamp(0.83rem, 0.80rem + 0.14vw, 0.90rem)`
- `--type-base`: `clamp(0.96rem, 0.92rem + 0.16vw, 1.04rem)`
- `--type-md`: `clamp(1.15rem, 1.08rem + 0.26vw, 1.25rem)`
- `--type-lg`: `clamp(1.38rem, 1.25rem + 0.46vw, 1.56rem)`
- `--type-xl`: `clamp(1.66rem, 1.43rem + 0.80vw, 1.95rem)`
- `--type-2xl`: `clamp(1.99rem, 1.65rem + 1.15vw, 2.44rem)`
- `--type-3xl`: `clamp(2.39rem, 1.85rem + 1.80vw, 3.05rem)`

### Color architecture

- Dominant, 60%: mineral canvas `#F4F2EC`.
- Secondary, 30%: white working surfaces `#FFFFFF`, structured by graphite rules and navigation.
- Accent, 10%: signal vermilion `#C83B2D` for primary actions, live state, and hard conflicts.

Text hierarchy on the mineral canvas:

- Primary graphite `#171915`: 15.81:1, WCAG AAA.
- Secondary graphite `#484C46`: 7.82:1, WCAG AAA.
- Muted graphite `#686D66`: 4.73:1, WCAG AA.
- Vermilion accent `#C83B2D`: 4.55:1, WCAG AA.
- White on graphite: 17.70:1, WCAG AAA.
- White on vermilion: 5.10:1, WCAG AA.

Track colors appear as narrow rails and labels rather than large fills. Success
uses forest; warnings use ochre; hard scheduling failures use vermilion. Soft
semantic backgrounds retain graphite text.

### Motion

Subtle. Navigation swaps, drawers, disclosure panels, and drag feedback use
motion, but tables and initial server-rendered content do not animate into
place. The system uses 160ms for feedback, 240ms for state changes, and 420ms
for large overlays. `ease-out-expo` handles entrances and expansion;
`ease-spring` is reserved for drag placement and toggles. Reduced-motion mode
removes transforms and makes all transitions effectively immediate.

### Spacing

An 8px base unit drives a fluid seven-step scale:

- `--space-xs`: `clamp(0.50rem, 0.46rem + 0.15vw, 0.625rem)`
- `--space-sm`: `clamp(0.75rem, 0.68rem + 0.25vw, 1rem)`
- `--space-md`: `clamp(1rem, 0.90rem + 0.35vw, 1.5rem)`
- `--space-lg`: `clamp(1.5rem, 1.32rem + 0.60vw, 2rem)`
- `--space-xl`: `clamp(2rem, 1.70rem + 1vw, 3rem)`
- `--space-2xl`: `clamp(3rem, 2.45rem + 1.80vw, 4rem)`
- `--space-3xl`: `clamp(4rem, 3rem + 3vw, 6rem)`

### Binding token contract

```css
:root {
  --font-display: "Space Grotesk", "Avenir Next", sans-serif;
  --font-body: "Work Sans", "Trebuchet MS", sans-serif;
  --font-mono: "IBM Plex Mono", "Courier New", monospace;
  --weight-regular: 400;
  --weight-medium: 500;
  --weight-semibold: 600;

  --type-xs: clamp(0.72rem, 0.69rem + 0.12vw, 0.78rem);
  --type-sm: clamp(0.83rem, 0.80rem + 0.14vw, 0.90rem);
  --type-base: clamp(0.96rem, 0.92rem + 0.16vw, 1.04rem);
  --type-md: clamp(1.15rem, 1.08rem + 0.26vw, 1.25rem);
  --type-lg: clamp(1.38rem, 1.25rem + 0.46vw, 1.56rem);
  --type-xl: clamp(1.66rem, 1.43rem + 0.80vw, 1.95rem);
  --type-2xl: clamp(1.99rem, 1.65rem + 1.15vw, 2.44rem);
  --type-3xl: clamp(2.39rem, 1.85rem + 1.80vw, 3.05rem);

  --leading-display: 1.1;
  --leading-control: 1.35;
  --leading-prose: 1.55;

  --color-canvas: #f4f2ec;
  --color-surface: #ffffff;
  --color-surface-muted: #e8e6df;
  --color-surface-strong: #171915;
  --color-text: #171915;
  --color-text-secondary: #484c46;
  --color-text-muted: #686d66;
  --color-text-inverse: #ffffff;
  --color-line: #d6d3ca;
  --color-line-strong: #a8a59c;
  --color-accent: #c83b2d;
  --color-accent-strong: #b83226;
  --color-accent-soft: #f2e6e2;
  --color-success: #2c5e50;
  --color-success-soft: #e2ece8;
  --color-warning: #7a4b00;
  --color-warning-soft: #f3ebd5;
  --color-track-blue: #355c8a;
  --color-track-violet: #6b4f7e;
  --color-track-ochre: #8a611d;
  --color-track-teal: #2c6b67;

  --space-xs: clamp(0.50rem, 0.46rem + 0.15vw, 0.625rem);
  --space-sm: clamp(0.75rem, 0.68rem + 0.25vw, 1rem);
  --space-md: clamp(1rem, 0.90rem + 0.35vw, 1.5rem);
  --space-lg: clamp(1.5rem, 1.32rem + 0.60vw, 2rem);
  --space-xl: clamp(2rem, 1.70rem + 1vw, 3rem);
  --space-2xl: clamp(3rem, 2.45rem + 1.80vw, 4rem);
  --space-3xl: clamp(4rem, 3rem + 3vw, 6rem);

  --duration-instant: 1ms;
  --duration-fast: 160ms;
  --duration-base: 240ms;
  --duration-slow: 420ms;
  --ease-out: cubic-bezier(0.16, 1, 0.3, 1);
  --ease-spring: cubic-bezier(0.34, 1.56, 0.64, 1);

  --radius-sm: 0.375rem;
  --radius-md: 0.625rem;
  --radius-lg: 1rem;
  --shadow-float: 0 1rem 3rem rgb(23 25 21 / 0.14);
  --content-wide: 100rem;
  --content-prose: 44rem;
  --sidebar-width: 15.5rem;
  --sidebar-collapsed: 4.75rem;
  --command-width: 42rem;
  --command-list-height: 27rem;
}
```

## Demo narrative

1. Open Today and show the event pulse, unresolved conflicts, review progress, and outstanding speaker work.
2. Open the CFP builder and preview the conditional workshop question and category routing policy.
3. Submit a proposal through the public form and verify the direct portal redirect and confirmation state.
4. Update the new speaker profile and complete an onboarding task.
5. Return to organizer mode, change the proposal status, and score it in a later review round.
6. Open Agenda, drag an accepted session into a conflicting slot, inspect the rule trace, and resolve it.
7. Preview an acceptance/reminder email and download its calendar invite.
8. Preview the Accelevents payload and the mobile agenda and speaker-gallery embeds.

## Acceptance constraints

- The critical demo path works without third-party credentials.
- External adapters fail closed and show actionable configuration state.
- All primary pages are server-rendered and usable before client enhancement.
- Internal navigation, managed mutations, and Hub-driven refreshes complete without a document reload when client enhancement is available.
- The organizer quick switcher is searchable and operable by touch, `Ctrl/Cmd K`, arrows, Enter, Escape, and backdrop click; route chords never fire inside typing controls.
- The desktop workspace rail reaches the declared 4.75 rem compact width and persists its preference; the mobile shell retains a visible switcher without exposing a non-functional collapse control.
- Drag/drop has keyboard-accessible move controls.
- Organizer pages remain usable at 1280 px wide; public and portal pages remain usable at 390 px wide.
- Bespoke browser JavaScript remains exactly zero files and zero bytes; the only JavaScript permitted in browser output is GoSX's generic runtime.
- CSS stays below 14,500 gzip bytes, the largest exported HTML route below 145,000 raw / 26,000 gzip bytes, and the largest binary island below 12,500 raw / 4,500 gzip / 3,500 Brotli bytes.
- The public root is navigation-only. Representative organizer and public-agenda routes must match exact island, Controller, Hub, bootstrap, and WASM capability sets; the largest route-scoped GoSX transfer stays below 420,000 gzip / 335,000 Brotli bytes.
- The committed cold-entry and island-route performance profiles pass under a Pixel 7 viewport and 4× CPU throttle.
- Every required capability has at least one automated or browser-level verification.
- README includes local run, production configuration, demo walkthrough, API, and deployment instructions.

## Deferred after the hackathon slice

- Submission fees and payment gateways.
- SMS delivery.
- Full multi-tenant billing and organization administration.
- Tenant-scoped identity, authorization, and multi-instance transactional storage.
- Background provider delivery workers and production Gmail/Outlook account connections.
- Rich document editing, document generation, and bulk archive jobs.
- Bidirectional Accelevents synchronization.
- A general-purpose CMS or CRM beyond speaker-program operations.

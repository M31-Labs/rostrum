---
description: The Paper & Ink typography, color, spacing, motion, component, and accessibility contract.
nav_order: "08 / 08"
eyebrow: Preserve Paper & Ink
---

# Rostrum design field guide

<!-- markdownlint-disable MD013 -->

## Visual System

**Paper & Ink** is Rostrum's editorial field guide: the calm authority of a
printed programme adapted to a dense operator workspace. It should feel
considered, tactile, and legible under pressure—not ornamental, glossy, or
dashboard-generic.

The source of truth is [`public/styles.css`](https://github.com/M31-Labs/rostrum/blob/main/public/styles.css). This guide
names the existing contract so new surfaces can join it without inventing a
second visual language.

## Design principles

1. **Editorial hierarchy over dashboard chrome.** Strong display headings,
   folio labels, rules, and deliberate whitespace create order before cards do.
2. **Warm surfaces, serious ink.** Paper tones reduce glare; deep forest text
   keeps contrast and authority. Ochre marks attention without shouting.
3. **Density with air.** Operator tables may be information-rich, but section
   spacing and typographic rhythm keep scanning calm.
4. **Status has language and shape.** Never rely on color alone. Pair tone with
   a readable label, count, icon, rule, or border.
5. **Motion confirms, never performs.** Transitions explain navigation,
   disclosure, and state changes. Reduced-motion preference wins.

## Typography

| Role | Family | Loaded weights | Use |
| --- | --- | --- | --- |
| Display | Fraunces, then Iowan Old Style/Georgia | 500, 600 | Page heroes, major section headings, editorial emphasis |
| Body and UI | Instrument Sans, then Avenir Next/Segoe UI | 400, 500, 600 | Prose, controls, navigation, table content |
| Folio and data | Spline Sans Mono, then Menlo | 400, 500 | Eyebrows, metadata, timestamps, keyboard hints, status labels |

The CSS type scale is fluid from `--type-xs` through `--type-hero`, using
`clamp()` to preserve hierarchy from mobile to wide workspace screens. Display
leading is `1.04`, tight headings `1.16`, controls `1.35`, and prose `1.55`.
Uppercase mono labels use tracked spacing (`0.08em` or `0.14em`) sparingly.

Do not synthesize unrequested bold weights. The loaded contract tops out at
600; emphasis comes from scale, family, spacing, color, and rules.

## Color

### Paper and forest foundation

| Token | Value | Purpose |
| --- | --- | --- |
| `--color-canvas` | `#f4f1e8` | Warm paper page ground |
| `--color-surface` | `#fcfbf6` | Raised reading surface |
| `--color-surface-muted` | `#ece8db` | Quiet grouping surface |
| `--color-surface-strong` | `#1e2a24` | Deep forest inverse field |
| `--color-text` | `#20291f` | Primary ink |
| `--color-text-secondary` | `#47524a` | Supporting copy |
| `--color-text-muted` | `#66716a` | Metadata that still meets the contrast contract |
| `--color-line` | `#dad5c6` | Paper rule |
| `--color-line-strong` | `#a9a493` | Structural rule |

### Ochre and semantic accents

| Token | Value | Purpose |
| --- | --- | --- |
| `--color-accent` | `#8a5b1c` | Primary burnished ochre |
| `--color-accent-strong` | `#6f4813` | Accessible ochre text/action |
| `--color-accent-bright` | `#b07e28` | Progress line and high-energy detail |
| `--color-accent-soft` | `#f0e6ce` | Ochre wash |
| `--color-danger` | `#a73a2a` | Blocking conflict or destructive action |
| `--color-success` | `#2b6e4f` | Positive completion/published state |
| `--color-warning` | `#8a6410` | Non-blocking attention |

Track colors—blue `#3468b2`, teal `#0e8163`, violet `#8a55b8`, and ochre
`#a96a00`—form a categorical set. Always repeat the track name in text.

The stylesheet records a contrast contract against the canvas: primary and
secondary ink exceed AAA targets; muted and deep ochre meet normal-text AA.
Preserve those pairings when introducing a new surface.

## Spacing and layout

Spacing is fluid and derived from an 8 px rhythm:

- Micro adjustments: 2 px and 4 px (`--space-3xs`, `--space-2xs`).
- Core rhythm: fluid 8–10, 12–16, and 16–24 px steps.
- Section rhythm: fluid 24–32, 32–48, 48–64, and 64–96 px steps.
- Page gutters scale from 16 px to 64 px.

Prefer the named tokens over one-off values. Use the core rhythm inside
controls and cards; use section rhythm between narrative regions. Tables may
scroll within a bounded panel rather than forcing the page wider than the
20 rem minimum viewport.

The workspace uses a 15.5 rem navigation rail (4.75 rem collapsed), a 4.5 rem
header, and 2.75 rem controls. Prose is capped at 44 rem; wide operational
content can reach 100 rem.

## Shape, rules, and elevation

- Borders are thin (1 px) or emphatic (3 px).
- Corners stay restrained at 3, 6, or 10 px. Pills are reserved for controls
  whose semantics need a capsule, not as a default card treatment.
- The signature editorial rule is a strong line paired with a fine rule.
- Small-caps mono folios and stamp-like statuses provide identity.
- Shadows are quiet: a 1 px lift or a broad 12 px/32 px float using forest at
  12% opacity.

## Motion

The implemented timing lanes are:

| Token | Duration | Use |
| --- | --- | --- |
| `--duration-instant` | 1 ms | Reduced-motion replacement and immediate state |
| `--duration-fast` | 160 ms | Hover, focus, color, small transform |
| `--duration-base` | 240 ms | Page entry, navigation rail, dialog entry |
| `--duration-slow` | 420 ms | Progress and deliberate loading affordances |

Default easing is `cubic-bezier(0.16, 1, 0.3, 1)`, an ease-out curve that
settles quickly. Small tactile transforms may use
`cubic-bezier(0.34, 1.56, 0.64, 1)`. Avoid continuous animation except for an
active progress or loading state.

At `prefers-reduced-motion: reduce`, smooth scroll is disabled and animation
and transition durations collapse to the instant token. New motion must honor
the same override.

## Component language

- **Workspace:** dark forest rail, paper canvas, editorial headings, hard-working
  tables, keyboard-first switcher.
- **Public event:** programme-like chronology, visible track legend, strong
  time/title hierarchy, device-local itinerary.
- **Forms:** labels before controls, help and validation adjacent to the field,
  generous target sizes, no placeholder-only labels.
- **Status:** a noun or action in plain language plus color; danger is reserved
  for blocking/destructive conditions.
- **Empty and failure states:** explain what happened and name the next safe
  action. Keep the voice calm and specific.

## Accessibility contract

- Maintain a visible focus treatment using the ochre focus wash.
- Preserve the skip link and semantic heading order.
- Keep every action keyboard reachable; drag interactions require equivalent
  move controls.
- Never encode track, state, or conflict in color alone.
- Respect reduced motion and print styles.
- Test from the 20 rem minimum viewport through the wide workspace layout.

## Review checklist

- Does the surface use the three existing type roles and loaded weights?
- Are colors referenced through tokens, with readable text equivalents?
- Is spacing drawn from the fluid 8 px-derived scale?
- Does motion use 160/240/420 ms lanes and the existing easing curves?
- Are focus, keyboard, reduced-motion, narrow-screen, and print states intact?
- Does the page feel like one printed programme, not a collection of generic
  dashboard cards?

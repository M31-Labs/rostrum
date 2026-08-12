# Contributing to Rostrum

Thanks for helping make complicated event programs calmer. Rostrum values
small, explainable changes that preserve operator trust.

## Before opening a change

1. Search existing issues and pull requests.
2. For a large feature or a change to a public contract, open a proposal issue
   before investing in implementation.
3. Keep secrets, signed speaker/reviewer links, organizer setup tokens, real
   event data, and provider credentials out of issues, fixtures, commits, and
   screenshots.

Use the repository's fictional M31 Systems Forum data for examples.

## Local setup

Requirements:

- Go 1.26 or newer
- GoSX v0.38.1
- Arbiter v1.9.0

```bash
go install m31labs.dev/gosx/cmd/gosx@v0.38.1
go install m31labs.dev/arbiter/cmd/arbiter@v1.9.0
cp .env.example .env
DEMO_MODE=memory go run .
```

Open the one-time `/setup?token=…` URL printed by the process to create your
disposable organizer session.

## Change guidelines

- Preserve one canonical workspace and the published-data allow-list.
- Put governed business decisions in the policy layer rather than scattering
  conditionals across routes.
- Keep human-readable rule and audit context when changing a decision flow.
- Give drag, pointer, and color interactions an equivalent keyboard/text path.
- Reuse the [Paper & Ink visual system](../docs/visual-system.md).
- Add or update tests at the layer whose contract changed.
- Document new environment variables in `.env.example` and operational effects
  in `docs/deployment.md`.
- Do not claim external-provider success from a fake server, dry run, or local
  outbox test.

## Verification

Run the full local gate before requesting review:

```bash
make check
make smoke
make size-budget
```

`make check` includes race tests. `make smoke` boots the deterministic
read-only fixture and verifies its complete organizer/persona/public contract.
`make size-budget` creates a production bundle and enforces committed
route/runtime limits.

If the change affects deployment, identity, email, an external database,
Airtable, imports, or recovery, describe the additional acceptance evidence
that an operator still needs.

## Pull requests

Keep the pull request focused. Explain:

- the user/operator problem;
- what changed and why;
- the trust or data boundary affected;
- verification performed;
- known risks, limitations, or follow-up.

Link the relevant issue and include screenshots for visible changes using
fictional data only. Do not commit generated `dist/`, local `.env`, workspace
data, uploads, or temporary QA output.

By contributing, you agree that your contribution is licensed under the
repository's [MIT License](../LICENSE).

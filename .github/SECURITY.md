# Security policy

## Reporting a vulnerability

Do not open a public issue for a suspected vulnerability and do not include
working secrets, signed links, setup tokens, real speaker data, or exploit data
in a public thread.

Use GitHub's private vulnerability reporting for this repository when it is
available. If that option is unavailable, ask
[M31 Labs](https://m31labs.dev/build) for a private security channel. Put only
contact details and a safe, high-level description in that first message; do
not paste exploit data, secrets, signed links, or real event data into the
contact form. In the private report, include:

- affected commit or release identifier;
- affected route or component;
- prerequisites and a minimal reproduction using fictional data;
- expected and observed security boundary;
- impact and any safe mitigation you have tested.

Please allow maintainers time to reproduce and coordinate a fix before public
disclosure. Maintainers will acknowledge through the same private channel;
this repository does not currently promise a public response-time SLA.

## Supported versions

Rostrum has not published a stable release line. Security work targets the
current `main` branch and the exact immutable deployment candidate named by an
operator. No older tag should be assumed supported unless a release notice
explicitly says so.

## Sensitive material

Treat all of the following as secrets or private event data:

- `SESSION_SECRET`, reset/setup tokens, OAuth secrets, database URLs, mail and
  Airtable credentials;
- signed reviewer, speaker portal, calendar, and private-file links;
- speaker email, proposal text, review content, uploads, exports, archives,
  backups, and audit ledgers.

The M31 Systems Forum fixture under `examples/demo/` is fictional and is the
only dataset intended for public examples. Its generated workspace and
checksum manifests are deployment-specific runtime artifacts, not production
defaults.

## Deployment boundary

Repository tests do not prove the security of an operator's TLS termination,
secret manager, external database, mail provider, backup storage, ingress, or
network policy. Start with the
[self-hosting manual](../docs/self-hosting.md), then use the
[deployment reference](../docs/deployment.md) and
[launch-readiness gate](../docs/launch-readiness.md) for those checks.

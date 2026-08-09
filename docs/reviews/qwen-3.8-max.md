# Buckley / Qwen 3.8 Max review

Reviewed: 2026-08-09

The requested Buckbot workflow is now exposed by Buckley 0.4 as `buckley
review`. The structured project review ran with model
`qwen/qwen3.8-max`, high reasoning, and a disposable Git snapshot of this
workspace.

## Model result

Qwen returned **B / WATCH** with low confidence. It proved no source-level
defects because its review sandbox made zero tool calls and could not execute
the build or test suite. Its three requested follow-ups were therefore treated
as verification tasks, not as confirmed defects.

| Review request | Disposition |
|---|---|
| Execute the full build and test suite | Resolved: `make check` passed formatting, 21 GoSX sources, three Arbiter policies, `go vet`, unit tests, and race tests. `gosx build --prod .` completed successfully. |
| Re-run structural analysis without generated `dist/` | Resolved: source-only Canopy analysis found 57 files, zero parse errors, and no unexplained dead application path. The low resolution rate is dominated by external package calls and GoSX runtime-registered actions/handlers. One genuinely unused helper was removed, and a registered-but-unexposed task approval action became a visible managed workflow. |
| Confirm `dist/` is not shipped as source | Resolved: `dist/` is ignored by Git but is now the verified input to the runtime-only Dockerfile. The reproducible production packaging step removes copied `.go` files and generated sourcemaps; the image selects the stripped binary, `.gsx` templates, hashed GoSX assets, and public/data files and runs as the non-root `programma` user. |

## Independent verification after review

- A 16-route browser audit found one `h1` and one `main` per page, no unnamed
  controls, missing descriptions, duplicate IDs, desktop overflow, or page
  errors.
- Internal links, managed form mutations, external Hub refreshes, and task
  approval all preserved the same document with one navigation entry.
- Native and server validation, inline field errors, focus recovery, agenda
  success and policy-blocked drag paths, dialog labeling, same-origin embeds,
  clipboard feedback, and itinerary recovery/persistence passed.
- Organizer and public layouts passed at 1024 px and 390 px respectively.
- The container health endpoint returned `ok: true` from the final image.

The original B grade is retained here as the model's unmodified output state;
the evidence above records how its unresolved checks were subsequently closed.

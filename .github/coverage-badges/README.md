# Coverage badge endpoint templates

Cold-start shields.io endpoint JSON for README coverage badges.

Live endpoints are published on the orphan `coverage-badges` branch
(`coverage-web.json`, `coverage-sandbox.json`) by `ci-web` / `ci-sandbox`
on default-branch success only.

Schema (shields Endpoint Badge):

```json
{
  "schemaVersion": 1,
  "label": "coverage-web",
  "message": "n/a",
  "color": "lightgrey"
}
```

Color bands (see `.github/scripts/coverage-badge-color.sh`):

| Lines % | Color  | Band |
|--------:|--------|------|
| ≥ 85    | green  | high |
| 70–84   | yellow | mid  |
| < 70    | orange | low  |
| n/a     | lightgrey | cold start |

Auth: workflows use the job `GITHUB_TOKEN` with `contents: write` to update
only the `coverage-badges` branch. No coverage SaaS and no `contents: write`
to `main`. A dedicated `GIST_TOKEN` is not required for this carrier.

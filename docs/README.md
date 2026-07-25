# Approving project site

Static HTML homepage + Markdown help, built to `public/` and published by
`ci-docs` to [`cocofhu/approving-pages`](https://github.com/cocofhu/approving-pages)
(`https://cocofhu.github.io/approving-pages/`).

## Layout

| Path | Role |
|------|------|
| `site/` | Static assets and homepage (`index.html`, CSS, JS) |
| `content/` | Help / guide Markdown (`*.md` with YAML front matter) |
| `scripts/build.mjs` | Copy `site/` → `public/`, render Markdown → HTML |
| `public/` | Build output (gitignored) |

## Local commands

```bash
cd docs
npm ci --no-audit --no-fund
npm run build                 # BASE_PATH=/approving-pages (Pages URL)
BASE_PATH=/ npm run server    # local preview at http://localhost:4000
```

## Deploy prerequisites

See [Contributing → Project site](../CONTRIBUTING.md#project-site-docs): create
`cocofhu/approving-pages`, enable GitHub Pages on `main`/root, and set repo
Secret `PAGES_DEPLOY_TOKEN` on `cocofhu/approving`.

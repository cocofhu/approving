# Approving project site

Static HTML homepage + Markdown help, built to `public/` and published by
`ci-docs` to [`cocofhu/approving-pages`](https://github.com/cocofhu/approving-pages)
(`https://www.approving-ai.com/`).

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
npm run build                 # BASE_PATH=/ (custom domain root)
BASE_PATH=/ npm run server    # local preview at http://localhost:4000
```

## Deploy prerequisites

See [Contributing → Project site](../CONTRIBUTING.md#project-site-docs): create
`cocofhu/approving-pages`, enable GitHub Pages on `main`/root, add a write
Deploy key on `approving-pages`, and store the private key as Secret
`PAGES_DEPLOY_KEY` on `cocofhu/approving`.

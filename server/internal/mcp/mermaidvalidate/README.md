# mermaidvalidate

`bundle.mjs` is a Node ESM bundle of Mermaid 11.x `parse()` used by `set_plan`
to reject illegal diagram sources before they land in `plan.json`.

Regenerate after bumping `web` mermaid (from repo root, with `web/node_modules` installed):

```bash
./server/internal/mcp/mermaidvalidate/regen.sh
```

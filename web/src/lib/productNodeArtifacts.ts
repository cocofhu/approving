// Single source for StructuredProductPanel renderable node types ↔ artifact names.
// Manifest products cover Agent deliverables; proposal_select is the confirmed pick.

import manifest from '@/data/nodeManifest.generated.json'

type ProductEntry = { type: string; artifactName: string }

const fromManifest = Object.fromEntries(
  (manifest.products as ProductEntry[]).map((p) => [p.type, p.artifactName]),
)

/** Reserved product artifact per node type (generic `agent` intentionally absent). */
export const PRODUCT_ARTIFACT_BY_TYPE: Record<string, string> = {
  ...fromManifest,
  // Confirmed single proposal — same card as proposals.json, highlighted as selected.
  proposal_select: 'proposal.json',
}

export const PRODUCT_NODE_TYPES: string[] = Object.keys(PRODUCT_ARTIFACT_BY_TYPE)

export function productArtifactName(nodeType: string): string | undefined {
  return PRODUCT_ARTIFACT_BY_TYPE[nodeType]
}

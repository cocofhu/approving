// Structured-product mappings — generated from server/internal/nodereg (single
// source of truth). Regenerate: `go run ./server/cmd/gen-nodereg/main.go`
// from repo root (or set APPROVING_ROOT).

import manifest from '@/data/nodeManifest.generated.json'

type NodeManifest = {
  outputKeyToArtifact: Record<string, string>
  artifactToOutputJSON: Record<string, string>
}

const m = manifest as NodeManifest

export const OUTPUT_KEY_TO_ARTIFACT: Record<string, string> = m.outputKeyToArtifact

export const ARTIFACT_TO_OUTPUT_JSON: Record<string, string> = m.artifactToOutputJSON

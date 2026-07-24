import { inferArtifactKind, isReadonlyArtifactKind } from '@/lib/gateUpstream'
import { isJsonArtifact } from '@/lib/highlightJson'
import { isStructuredArtifactName } from './StructuredArtifactView.vue'

export type ArtifactPreviewBranch =
  | { kind: 'html' }
  | { kind: 'structured'; doc: unknown }
  | { kind: 'json' }
  | { kind: 'image' }
  | { kind: 'markdown' }
  | { kind: 'empty' }

export function isImagePreviewArtifact(name: string, kind?: string | null): boolean {
  return isReadonlyArtifactKind(kind ?? undefined) || inferArtifactKind(name) === 'image'
}

/**
 * Shared preview routing for ArtifactPreview inline + zoom modal.
 * Priority: HTML → valid reserved structured JSON → JSON/source → Image → Markdown → empty.
 * Independent of platform/run scope so both entry points stay consistent.
 */
export function resolveArtifactPreviewBranch(input: {
  name: string
  kind?: string | null
  content: string
}): ArtifactPreviewBranch {
  const isHtml = input.kind === 'html'
  const content = input.content
  if (isHtml && content) return { kind: 'html' }

  if (isStructuredArtifactName(input.name)) {
    const raw = content.trim()
    if (raw) {
      try {
        return { kind: 'structured', doc: JSON.parse(raw) as unknown }
      } catch {
        /* fall through to JSON/source diagnostic view */
      }
    }
  }

  if (isJsonArtifact({ kind: input.kind as any, name: input.name }) && content) {
    return { kind: 'json' }
  }
  if (isImagePreviewArtifact(input.name, input.kind)) {
    return { kind: 'image' }
  }
  if (content) return { kind: 'markdown' }
  return { kind: 'empty' }
}

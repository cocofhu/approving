import { describe, expect, it } from 'vitest'
import { isStructuredArtifactName } from './StructuredArtifactView.vue'

describe('isStructuredArtifactName', () => {
  it('matches the 8 reserved structured JSON artifact names', () => {
    const names = [
      'clarified_requirement.json',
      'research.json',
      'proposals.json',
      'proposal.json',
      'plan.json',
      'implementation_result.json',
      'test_result.json',
      'review.json',
    ]
    for (const name of names) {
      expect(isStructuredArtifactName(name)).toBe(true)
    }
  })

  it('does not match markdown, html, or other artifact names', () => {
    expect(isStructuredArtifactName('clarified_requirement.md')).toBe(false)
    expect(isStructuredArtifactName('page.html')).toBe(false)
    expect(isStructuredArtifactName('design.md')).toBe(false)
    expect(isStructuredArtifactName('result.json')).toBe(false)
  })
})

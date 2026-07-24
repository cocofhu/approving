import { describe, expect, it } from 'vitest'
import { PRODUCT_ARTIFACT_BY_TYPE, PRODUCT_NODE_TYPES, productArtifactName } from './productNodeArtifacts'

describe('productNodeArtifacts', () => {
  it('covers StructuredProductPanel node types including proposal_select', () => {
    expect(PRODUCT_NODE_TYPES).toEqual(expect.arrayContaining(['react', 'research', 'plan', 'proposal_select', 'visual']))
    expect(productArtifactName('research')).toBe('research.json')
    expect(productArtifactName('proposal_select')).toBe('proposal.json')
    expect(PRODUCT_ARTIFACT_BY_TYPE.react).toBe('clarified_requirement.json')
  })
})

import { describe, expect, it } from 'vitest'
import { buildFrontmatter, frontmatterTypeForPath, parseFrontmatter } from './frontmatter'

describe('frontmatter', () => {
  it('parses rules frontmatter and body', () => {
    const text = '---\ndescription: hello\nalwaysApply: true\n---\n\n# Title\n'
    const { fm, body } = parseFrontmatter(text)
    expect(fm?.description).toBe('hello')
    expect(fm?.alwaysApply).toBe(true)
    expect(body).toBe('\n# Title\n')
  })

  it('builds skill frontmatter', () => {
    const built = buildFrontmatter({ name: 'commit', description: 'desc' }, 'skill')
    expect(built).toContain('name: "commit"')
    expect(built).toContain('description: "desc"')
  })

  it('detects path types', () => {
    expect(frontmatterTypeForPath('rules/foo.md')).toBe('rules')
    expect(frontmatterTypeForPath('skills/commit/SKILL.md')).toBe('skill')
    expect(frontmatterTypeForPath('other.md')).toBeNull()
  })
})

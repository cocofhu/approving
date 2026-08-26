import { describe, expect, it } from 'vitest'
import { classifyDiffLine, DIFF_LINE_CLASS, revisionMatchesFile } from './workspaceHistoryDiff'

describe('workspaceHistoryDiff', () => {
  it('classifies unified diff lines', () => {
    expect(classifyDiffLine('@@ -1,3 +1,4 @@')).toBe('hunk')
    expect(classifyDiffLine('--- a/file.md')).toBe('file')
    expect(classifyDiffLine('+++ b/file.md')).toBe('file')
    expect(classifyDiffLine('+ added')).toBe('add')
    expect(classifyDiffLine('- removed')).toBe('del')
    expect(classifyDiffLine(' context')).toBe('context')
  })

  it('maps diff line kinds to theme classes', () => {
    expect(DIFF_LINE_CLASS.add).toContain('text-ok')
    expect(DIFF_LINE_CLASS.del).toContain('text-err')
  })

  it('matches revisions by path or fromPath', () => {
    const changes = [{ path: 'rules/role.md' }]
    expect(revisionMatchesFile(changes, 'rules/role.md')).toBe(true)
    expect(revisionMatchesFile(changes, 'AGENTS.md')).toBe(false)
    expect(revisionMatchesFile([{ path: 'new.md', fromPath: 'old.md' }], 'old.md')).toBe(true)
    expect(revisionMatchesFile(undefined, 'x')).toBe(false)
    expect(revisionMatchesFile(changes, '')).toBe(false)
  })
})

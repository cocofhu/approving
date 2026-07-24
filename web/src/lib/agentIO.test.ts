import { describe, expect, it } from 'vitest'
import {
  peekAgentZipName,
  resolveImportName,
  suggestRename,
  validateAgentName,
} from './agentIO'

/** ZIP produced by Go archive/zip (agent.json stored; rules/*.md may be deflate). */
const GO_EXPORT_ZIP_B64 =
  'UEsDBBQACAAAAAAAAAAAAAAAAAAAAAAAAAAKAAAAYWdlbnQuanNvbnsKICAibmFtZSI6ICJwZWVrX3Rlc3QiLAogICJtY3AiOiBbCiAgICB7CiAgICAgICJuYW1lIjogImFydGlmYWN0LXN0b3JlIiwKICAgICAgInVybCI6ICIke0NPREVGTE9XX0FSVElGQUNUX1VSTH0iLAogICAgICAiaGVhZGVycyI6IHsKICAgICAgICAiQXV0aG9yaXphdGlvbiI6ICJCZWFyZXIgJHtDT0RFRkxPV19BUlRJRkFDVF9UT0tFTn0iCiAgICAgIH0KICAgIH0KICBdLAogICJsYXlvdXQiOiB7CiAgICAiY29uZmlnUm9vdCI6ICIvcm9vdC8uY3Vyc29yIiwKICAgICJ3b3Jrc3BhY2VEaXIiOiAiL3Jvb3Qvd29ya3NwYWNlIgogIH0sCiAgInNjaGVtYVZlcnNpb24iOiAxLAogICJleHBvcnRlZEF0IjogIjIwMjYtMDctMDlUMTE6MTU6MDRaIgp9UEsHCMzkgm5wAQAAcAEAAFBLAwQUAAgACAAAAAAAAAAAAAAAAAAAAAAACgAAAHJ1bGVzL2EubWRSVkgEBAAA//9QSwcIx1R7agkAAAADAAAAUEsBAhQAFAAIAAAAAAAAAMzkgm5wAQAAcAEAAAoAAAAAAAAAAAAAAAAAAAAAAGFnZW50Lmpzb25QSwECFAAUAAgACAAAAAAAx1R7agkAAAADAAAACgAAAAAAAAAAAAAAAACoAQAAcnVsZXMvYS5tZFBLBQYAAAAAAgACAHAAAADpAQAAAAA='

function goExportFile(): File {
  const bytes = Uint8Array.from(atob(GO_EXPORT_ZIP_B64), (c) => c.charCodeAt(0))
  return new File([bytes], 'ignored.zip', { type: 'application/zip' })
}

describe('validateAgentName', () => {
  it('accepts platform names', () => {
    expect(validateAgentName('Agent_1-test')).toBe('')
  })

  it('rejects invalid names', () => {
    expect(validateAgentName('bad name')).toBe('invalid')
    expect(validateAgentName('')).toBe('required')
  })
})

describe('resolveImportName', () => {
  it('prefers agent.json name over zip filename', () => {
    expect(resolveImportName('from_json', 'other.zip')).toBe('from_json')
  })

  it('falls back to zip filename', () => {
    expect(resolveImportName(undefined, 'fallback.zip')).toBe('fallback')
  })
})

describe('suggestRename', () => {
  it('picks first free _vN suffix', () => {
    expect(suggestRename('agent', ['agent', 'agent_v2'])).toBe('agent_v3')
  })
})

describe('peekAgentZipName', () => {
  it('reads name from Go-exported ZIP without hanging', async () => {
    const result = await Promise.race([
      peekAgentZipName(goExportFile()),
      new Promise<never>((_, reject) => setTimeout(() => reject(new Error('timeout')), 3000)),
    ])
    expect(result.error).toBeUndefined()
    expect(result.name).toBe('peek_test')
  })

  it('reports missing agent.json', async () => {
    const empty = new Uint8Array([0x50, 0x4b, 0x05, 0x06, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0])
    const file = new File([empty], 'empty.zip', { type: 'application/zip' })
    const result = await peekAgentZipName(file)
    expect(result.error).toBe('missing agent.json')
  })
})

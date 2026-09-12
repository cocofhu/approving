import { describe, expect, it } from 'vitest'
import type { Agent } from '@/lib/api/api'
import {
  draftPayloadJson,
  draftPromptsToApi,
  fromDraft,
  fromDraftRaw,
  hydrateStudioDraft,
  kvToRec,
  normalizePromptText,
  recToKV,
  PROMPT_KEYS,
  toDraft,
} from './agentStudioDraft'

const baseAgent: Agent = {
  name: '综合代码审查工程师',
  projectId: 'p1',
  acpBackend: 'cursor',
  files: [],
  mcp: [],
  env: {},
  layout: { configRoot: '/root/.cursor', workspaceDir: '/root/workspace' },
}

describe('legacy APPROVING_ env keys', () => {
  it('hydrates stored APPROVING_CURSOR_API_KEY as GRASP_CURSOR_API_KEY', () => {
    const d = hydrateStudioDraft({
      ...baseAgent,
      env: { APPROVING_CURSOR_API_KEY: 'crsr_old', FEATURE_FLAG: '1' },
    })
    expect(kvToRec(d.env)).toEqual({
      GRASP_CURSOR_API_KEY: 'crsr_old',
      FEATURE_FLAG: '1',
    })
    expect(fromDraft(d).env).toEqual({
      GRASP_CURSOR_API_KEY: 'crsr_old',
      FEATURE_FLAG: '1',
    })
  })

  it('folds APPROVING_ onto existing GRASP_ without clobbering', () => {
    expect(
      recToKV({
        GRASP_CURSOR_API_KEY: 'new',
        APPROVING_CURSOR_API_KEY: 'old',
      }),
    ).toEqual([{ k: 'GRASP_CURSOR_API_KEY', v: 'new' }])
    expect(
      kvToRec([
        { k: 'APPROVING_CURSOR_API_KEY', v: 'old' },
        { k: 'GRASP_CURSOR_API_KEY', v: 'new' },
      ]),
    ).toEqual({ GRASP_CURSOR_API_KEY: 'new' })
  })
})

describe('prompt dirty serialization (g1.1 / g1.2)', () => {
  it('normalizePromptText maps CRLF and CR to LF', () => {
    expect(normalizePromptText('a\r\nb\rc')).toBe('a\nb\nc')
  })

  it('omits empty and whitespace-only prompts the same as missing prompts', () => {
    expect(draftPromptsToApi({
      upstreamArtifactsHeader: '',
      producesContract: '  \n',
      reactOpenSuffix: '',
      producesRetry: '',
    })).toBeUndefined()
    expect(draftPromptsToApi(toDraft({ ...baseAgent }).prompts)).toBeUndefined()
  })

  it('toDraft hydrates missing prompts as empty strings with LF', () => {
    const d = toDraft({
      ...baseAgent,
      prompts: { producesContract: 'hello\r\nworld' },
    })
    expect(d.prompts.producesContract).toBe('hello\nworld')
    expect(d.prompts.upstreamArtifactsHeader).toBe('')
  })

  it('fromDraft and fromDraftRaw agree on prompts; fromDraft is the dirty baseline', () => {
    const d = hydrateStudioDraft({
      ...baseAgent,
      env: { GIT_SSH_PRIVATE_KEY: 'secret', FOO: '1' },
      prompts: { reactOpenSuffix: 'x\r\ny' },
    })
    const raw = fromDraftRaw(d)
    const canonical = fromDraft(d)
    expect(raw.prompts).toEqual(canonical.prompts)
    expect(canonical.env?.GIT_SSH_PRIVATE_KEY).toBeUndefined()
    expect(raw.env?.GIT_SSH_PRIVATE_KEY).toBe('secret')
    expect(JSON.parse(draftPayloadJson(d))).toEqual(canonical)
  })

  it('textarea-style LF writeback does not change canonical payload', () => {
    const d = hydrateStudioDraft({
      ...baseAgent,
      prompts: { producesRetry: 'line1\r\nline2' },
    })
    const before = draftPayloadJson(d)
    for (const k of PROMPT_KEYS) {
      d.prompts[k] = normalizePromptText(d.prompts[k])
    }
    expect(draftPayloadJson(d)).toBe(before)
  })
})

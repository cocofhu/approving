// @vitest-environment happy-dom
import { afterEach, describe, expect, it } from 'vitest'
import {
  HOME_DRAFT_ID,
  __resetDraftIdbForTests,
  __setDraftIdbBackendForTests,
  base64ToBlob,
  blobToBase64,
  createMemoryDraftIdb,
  getDraftIdb,
  isQuotaError,
  tryBase64ToBlob,
} from './draftIdb'

describe('draftIdb', () => {
  afterEach(() => {
    __resetDraftIdbForTests()
  })

  it('round-trips home and run drafts in the memory backend', async () => {
    const mem = createMemoryDraftIdb()
    __setDraftIdbBackendForTests(mem)
    const db = getDraftIdb()
    const blob = new Blob(['hello'], { type: 'text/plain' })
    await db.putHome(
      {
        id: HOME_DRAFT_ID,
        schemaVersion: '1',
        savedAt: 1,
        pipelineId: 'wf-1',
        text: 'draft',
      },
      [
        {
          id: 'att-1',
          ownerKind: 'home',
          ownerId: HOME_DRAFT_ID,
          mimeType: 'text/plain',
          data: blob,
          sortIndex: 0,
          name: 'a.txt',
        },
      ],
    )
    const home = await db.getHome()
    expect(home?.record.text).toBe('draft')
    expect(home?.attachments).toHaveLength(1)
    await db.deleteHome()
    expect(await db.getHome()).toBeNull()

    await db.putRun({ workflowId: 'wf-1', savedAt: 2, inputsJson: '{}' }, [])
    expect((await db.getRun('wf-1'))?.record.inputsJson).toBe('{}')
    await db.deleteRun('wf-1')
    expect(await db.getRun('wf-1')).toBeNull()
  })

  it('encodes blobs and treats quota errors', async () => {
    const blob = new Blob(['xyz'], { type: 'text/plain' })
    const b64 = await blobToBase64(blob)
    const round = base64ToBlob(b64, 'text/plain')
    expect(await round.text()).toBe('xyz')
    expect(tryBase64ToBlob('%%%', 'text/plain')).toBeNull()
    expect(isQuotaError({ name: 'QuotaExceededError' })).toBe(true)
    expect(isQuotaError({ code: 22 })).toBe(true)
    expect(isQuotaError({})).toBe(false)
  })
})

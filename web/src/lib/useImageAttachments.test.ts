// @vitest-environment happy-dom
import { describe, expect, it, vi } from 'vitest'
import { useImageAttachments } from './useImageAttachments'
import { SITE_ATTACH_MAX_BYTES } from './attachments'

describe('useImageAttachments', () => {
  it('adds any file type with name and rejects oversized at select', async () => {
    const { attachments, addFiles, onPaste, removeAttachment, clearAttachments, takeAttachments, onPickFiles, fileInput, notice, blockSendIfOversized } =
      useImageAttachments()

    class FakeReader {
      result: string | ArrayBuffer | null = null
      onload: null | (() => void) = null
      readAsDataURL(file: File) {
        this.result = `data:${file.type || 'application/octet-stream'};base64,QUJD`
        queueMicrotask(() => this.onload?.())
      }
    }
    vi.stubGlobal('FileReader', FakeReader as unknown as typeof FileReader)

    const img = new File(['ABC'], 'a.png', { type: 'image/png' })
    const pdf = new File(['ABC'], 'doc.pdf', { type: 'application/pdf' })
    const dt = new DataTransfer()
    dt.items.add(img)
    dt.items.add(pdf)
    addFiles(dt.files)
    await Promise.resolve()
    expect(attachments.value).toHaveLength(2)
    expect(attachments.value[0].mimeType).toBe('image/png')
    expect(attachments.value[0].name).toBe('a.png')
    expect(attachments.value[1].mimeType).toBe('application/pdf')
    expect(attachments.value[1].name).toBe('doc.pdf')

    // Oversized rejected at select; not added to pending.
    const huge = new File([new Uint8Array(SITE_ATTACH_MAX_BYTES + 1)], 'huge.bin', {
      type: 'application/octet-stream',
    })
    const overDt = new DataTransfer()
    overDt.items.add(huge)
    addFiles(overDt.files)
    await Promise.resolve()
    expect(attachments.value).toHaveLength(2)
    expect(notice.value?.kind).toBe('error')
    expect(notice.value?.text).toContain('50 MiB')
    expect(notice.value?.text).toContain('huge.bin')

    fileInput.value = document.createElement('input')
    onPickFiles({ target: { files: dt.files } } as unknown as Event)
    await Promise.resolve()

    const pasteDt = new DataTransfer()
    pasteDt.items.add(img)
    const preventDefault = vi.fn()
    onPaste({
      clipboardData: pasteDt,
      preventDefault,
    } as unknown as ClipboardEvent)
    await Promise.resolve()
    expect(preventDefault).toHaveBeenCalled()

    onPaste({ clipboardData: null } as unknown as ClipboardEvent)

    removeAttachment(0)
    expect(attachments.value.length).toBeGreaterThanOrEqual(0)
    clearAttachments()
    expect(attachments.value).toEqual([])

    attachments.value = [{ data: 'z', mimeType: 'image/jpeg', name: 'z.jpg' }]
    expect(takeAttachments()).toEqual([{ data: 'z', mimeType: 'image/jpeg', name: 'z.jpg' }])
    expect(attachments.value).toEqual([])

    // Send-stage gate: synthetic oversized base64 blocks send.
    const overB64 = 'A'.repeat(Math.ceil(((SITE_ATTACH_MAX_BYTES + 1024) * 4) / 3))
    attachments.value = [{ data: overB64, mimeType: 'application/octet-stream', name: 'over.bin' }]
    expect(blockSendIfOversized()).toBe(true)
    expect(notice.value?.text).toContain('发送已阻止')
    expect(notice.value?.text).toContain('over.bin')

    addFiles(null)
  })
})

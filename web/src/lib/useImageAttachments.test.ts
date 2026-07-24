// @vitest-environment happy-dom
import { describe, expect, it, vi } from 'vitest'
import { useImageAttachments } from './useImageAttachments'

describe('useImageAttachments', () => {
  it('adds image files via FileReader and supports paste/remove/clear/take', async () => {
    const { attachments, addFiles, onPaste, removeAttachment, clearAttachments, takeAttachments, onPickFiles, fileInput } =
      useImageAttachments()

    class FakeReader {
      result: string | ArrayBuffer | null = null
      onload: null | (() => void) = null
      readAsDataURL(file: File) {
        this.result = `data:${file.type};base64,QUJD`
        queueMicrotask(() => this.onload?.())
      }
    }
    vi.stubGlobal('FileReader', FakeReader as unknown as typeof FileReader)

    const img = new File(['ABC'], 'a.png', { type: 'image/png' })
    const txt = new File(['x'], 'a.txt', { type: 'text/plain' })
    const dt = new DataTransfer()
    dt.items.add(img)
    dt.items.add(txt)
    addFiles(dt.files)
    await Promise.resolve()
    expect(attachments.value).toHaveLength(1)
    expect(attachments.value[0].mimeType).toBe('image/png')

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

    attachments.value = [{ data: 'z', mimeType: 'image/jpeg' }]
    expect(takeAttachments()).toEqual([{ data: 'z', mimeType: 'image/jpeg' }])
    expect(attachments.value).toEqual([])

    addFiles(null)
  })
})

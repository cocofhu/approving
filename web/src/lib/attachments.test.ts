import { describe, expect, it } from 'vitest'
import {
  SITE_ATTACH_MAX_BYTES,
  attachmentByteLength,
  attachmentDisplayName,
  findOversizedAttachments,
  formatSelectRejectMessage,
  formatSendRejectMessage,
  isImageAttachment,
} from './attachments'

describe('attachments helpers', () => {
  it('classifies image vs non-image and preserves display names', () => {
    expect(isImageAttachment({ mimeType: 'image/png', name: 'a.png' })).toBe(true)
    expect(isImageAttachment({ mimeType: 'application/pdf', name: 'doc.pdf' })).toBe(false)
    expect(attachmentDisplayName({ data: 'x', mimeType: 'application/pdf', name: '需求.pdf' })).toBe('需求.pdf')
    expect(attachmentDisplayName({ data: 'x', mimeType: 'application/pdf' }, 2)).toBe('attachment-3')
  })

  it('detects oversized base64 payloads for send-stage gate', () => {
    const overB64 = 'A'.repeat(Math.ceil(((SITE_ATTACH_MAX_BYTES + 1024) * 4) / 3))
    const over = { data: overB64, mimeType: 'application/octet-stream', name: 'big.bin' }
    expect(attachmentByteLength(over)).toBeGreaterThan(SITE_ATTACH_MAX_BYTES)
    expect(findOversizedAttachments([over])).toHaveLength(1)
    expect(formatSelectRejectMessage(['big.bin'])).toContain('50 MiB')
    expect(formatSendRejectMessage(['big.bin'])).toContain('发送已阻止')
  })
})

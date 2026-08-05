import type { ClarifyImage } from '@/lib/types'

/** Site / platform ACP single-file limit (clarified: 50 MiB). */
export const SITE_ATTACH_MAX_BYTES = 50 * 1024 * 1024

/** QQ inbound single-file limit (clarified: 20 MiB). Documented for parity. */
export const QQ_ATTACH_MAX_BYTES = 20 * 1024 * 1024

export const SITE_ATTACH_MAX_MIB = 50

/** True when mime or filename looks like an image (preview as thumb). */
export function isImageAttachment(im: Pick<ClarifyImage, 'mimeType' | 'name'> | { mimeType?: string; name?: string }): boolean {
  const mime = (im.mimeType || '').toLowerCase()
  if (mime.startsWith('image/')) return true
  const name = (im.name || '').toLowerCase()
  return /\.(png|jpe?g|gif|webp|bmp|svg)$/i.test(name)
}

/** Display label: original name, else fallback. */
export function attachmentDisplayName(im: ClarifyImage, index = 0): string {
  const named = (im.name || '').trim()
  if (named) return named
  return `attachment-${index + 1}`
}

/** Prefer File.name; empty → distinguishable fallback. */
export function fileAttachmentName(file: File, index = 0): string {
  const named = (file.name || '').trim()
  if (named) return named
  return `attachment-${index + 1}`
}

/** Decode base64 payload length (approx original bytes). */
export function attachmentByteLength(im: ClarifyImage): number {
  if (!im.data) return 0
  const len = im.data.length
  let pad = 0
  if (im.data.endsWith('==')) pad = 2
  else if (im.data.endsWith('=')) pad = 1
  return Math.max(0, Math.floor((len * 3) / 4) - pad)
}

export function isOversizedAttachment(im: ClarifyImage, maxBytes = SITE_ATTACH_MAX_BYTES): boolean {
  return attachmentByteLength(im) > maxBytes
}

export function findOversizedAttachments(
  images: ClarifyImage[],
  maxBytes = SITE_ATTACH_MAX_BYTES,
): ClarifyImage[] {
  return images.filter((im) => isOversizedAttachment(im, maxBytes))
}

/** Human-readable reject toast: selection stage. */
export function formatSelectRejectMessage(names: string[], maxMiB = SITE_ATTACH_MAX_MIB): string {
  return `已拒绝超限文件：${names.join(', ')}。单文件上限 ${maxMiB} MiB，未进入待发送列表。`
}

/** Human-readable reject toast: send stage (blocks whole message). */
export function formatSendRejectMessage(names: string[], maxMiB = SITE_ATTACH_MAX_MIB): string {
  return `发送已阻止：存在超限文件（${names.join(', ')}）。上限 ${maxMiB} MiB，整条消息未发送。`
}

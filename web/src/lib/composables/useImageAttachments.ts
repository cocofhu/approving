import { ref } from 'vue'
import type { ClarifyImage } from '@/lib/shared/types'
import {
  SITE_ATTACH_MAX_BYTES,
  SITE_ATTACH_MAX_MIB,
  fileAttachmentName,
  findOversizedAttachments,
  formatSelectRejectMessage,
  formatSendRejectMessage,
  attachmentDisplayName,
} from '@/lib/shared/attachments'

export type AttachNotice = { kind: 'error' | 'ok'; text: string } | null

/** Shared paste/upload/preview/delete attachment logic (any type + 50 MiB gate). */
export function useImageAttachments(opts?: { maxBytes?: number; maxMiB?: number }) {
  const maxBytes = opts?.maxBytes ?? SITE_ATTACH_MAX_BYTES
  const maxMiB = opts?.maxMiB ?? SITE_ATTACH_MAX_MIB
  const attachments = ref<ClarifyImage[]>([])
  const fileInput = ref<HTMLInputElement | null>(null)
  const notice = ref<AttachNotice>(null)

  function setNotice(kind: 'error' | 'ok', text: string) {
    notice.value = { kind, text }
  }

  function clearNotice() {
    notice.value = null
  }

  function addFiles(files: FileList | null | undefined) {
    if (!files) return
    const rejected: string[] = []
    let accepted = 0
    const list = Array.from(files)
    list.forEach((f, i) => {
      if (f.size > maxBytes) {
        rejected.push(fileAttachmentName(f, i))
        return
      }
      const name = fileAttachmentName(f, i)
      const mimeType = f.type || 'application/octet-stream'
      const reader = new FileReader()
      reader.onload = () => {
        const res = String(reader.result || '')
        const comma = res.indexOf(',')
        attachments.value.push({
          data: comma >= 0 ? res.slice(comma + 1) : res,
          mimeType,
          name,
        })
      }
      reader.readAsDataURL(f)
      accepted++
    })
    if (rejected.length) {
      setNotice('error', formatSelectRejectMessage(rejected, maxMiB))
      return
    }
    if (accepted) clearNotice()
  }

  function onPickFiles(e: Event) {
    addFiles((e.target as HTMLInputElement).files)
    if (fileInput.value) fileInput.value.value = ''
  }

  function onPaste(e: ClipboardEvent) {
    const items = e.clipboardData?.items
    if (!items) return
    const picked: File[] = []
    for (const it of Array.from(items)) {
      // Paste: keep image clipboard items; non-image paste is typically text.
      if (it.kind === 'file') {
        const f = it.getAsFile()
        if (f) picked.push(f)
      }
    }
    if (picked.length) {
      e.preventDefault()
      const dt = new DataTransfer()
      picked.forEach((f) => dt.items.add(f))
      addFiles(dt.files)
    }
  }

  function removeAttachment(i: number) {
    attachments.value.splice(i, 1)
  }

  function clearAttachments() {
    attachments.value = []
  }

  function takeAttachments(): ClarifyImage[] {
    const imgs = attachments.value.slice()
    attachments.value = []
    return imgs
  }

  /** Returns oversized names if send should be blocked; empty = ok. */
  function validateForSend(images: ClarifyImage[] = attachments.value): string[] {
    return findOversizedAttachments(images, maxBytes).map((im, i) => attachmentDisplayName(im, i))
  }

  function blockSendIfOversized(images: ClarifyImage[] = attachments.value): boolean {
    const names = validateForSend(images)
    if (!names.length) return false
    setNotice('error', formatSendRejectMessage(names, maxMiB))
    return true
  }

  return {
    attachments,
    fileInput,
    notice,
    addFiles,
    onPickFiles,
    onPaste,
    removeAttachment,
    clearAttachments,
    takeAttachments,
    validateForSend,
    blockSendIfOversized,
    clearNotice,
    setNotice,
    maxBytes,
    maxMiB,
  }
}

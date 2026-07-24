import { ref } from 'vue'
import type { ClarifyImage } from '@/lib/types'

/** Shared paste/upload/preview/delete image attachment logic (ClarifyChat parity). */
export function useImageAttachments() {
  const attachments = ref<ClarifyImage[]>([])
  const fileInput = ref<HTMLInputElement | null>(null)

  function addFiles(files: FileList | null | undefined) {
    if (!files) return
    for (const f of Array.from(files)) {
      if (!f.type.startsWith('image/')) continue
      const reader = new FileReader()
      reader.onload = () => {
        const res = String(reader.result || '')
        const comma = res.indexOf(',')
        attachments.value.push({ data: comma >= 0 ? res.slice(comma + 1) : res, mimeType: f.type })
      }
      reader.readAsDataURL(f)
    }
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
      if (it.type.startsWith('image/')) {
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

  return {
    attachments,
    fileInput,
    addFiles,
    onPickFiles,
    onPaste,
    removeAttachment,
    clearAttachments,
    takeAttachments,
  }
}

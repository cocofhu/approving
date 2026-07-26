import { toCanvas } from 'html-to-image'
import { jsPDF } from 'jspdf'

/** Default wait for test_result screenshot placeholders before exporting. */
export const TEST_SCREENSHOT_WAIT_MS = 5000

/** Soft browser canvas limits; downscale when exceeded. */
const MAX_CANVAS_SIDE = 16384
const MAX_CANVAS_AREA = 268_435_456

export type StructuredExportFormat = 'png' | 'pdf'

export type StructuredExportResult = {
  filename: string
  incomplete: boolean
}

/** Strip the last extension and sanitize path-illegal characters (does not force .json). */
export function exportBasenameFromArtifactName(name: string): string {
  const raw = String(name || 'artifact').trim() || 'artifact'
  const withoutExt = raw.replace(/\.[^.]+$/, '') || 'artifact'
  return withoutExt.replace(/[^\w\u4e00-\u9fff\- .]/g, '_').replace(/[/\\]/g, '_')
}

export function exportFilename(artifactName: string, ext: StructuredExportFormat): string {
  return `${exportBasenameFromArtifactName(artifactName)}.${ext}`
}

/** Resolve current theme base color from CSS tokens (rgb channels on --c-base). */
export function resolveThemeBackgroundColor(el: Element = document.documentElement): string {
  const raw = getComputedStyle(el).getPropertyValue('--c-base').trim()
  if (raw) return `rgb(${raw})`
  return 'rgb(10, 10, 11)'
}

/** Trigger a browser download from a Blob (mirrors downloadJson pattern). */
export function triggerBlobDownload(filename: string, blob: Blob): void {
  const a = document.createElement('a')
  const url = URL.createObjectURL(blob)
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}

/**
 * Wait until test_result screenshot shimmers leave the export root, or timeout.
 * @returns true if ready (or no loading placeholders); false if timed out while still loading.
 */
export async function waitForTestScreenshots(
  root: HTMLElement,
  timeoutMs = TEST_SCREENSHOT_WAIT_MS,
): Promise<boolean> {
  const hasLoading = () => !!root.querySelector('.shot-shimmer')
  if (!hasLoading()) return true

  const start = Date.now()
  return new Promise((resolve) => {
    const tick = () => {
      if (!hasLoading()) {
        resolve(true)
        return
      }
      if (Date.now() - start >= timeoutMs) {
        resolve(false)
        return
      }
      window.setTimeout(tick, 50)
    }
    window.setTimeout(tick, 50)
  })
}

function computePixelRatio(width: number, height: number): number {
  let pixelRatio = Math.min(2, typeof window !== 'undefined' ? window.devicePixelRatio || 1 : 1)
  while (
    pixelRatio > 0.25 &&
    (width * pixelRatio > MAX_CANVAS_SIDE ||
      height * pixelRatio > MAX_CANVAS_SIDE ||
      width * height * pixelRatio * pixelRatio > MAX_CANVAS_AREA)
  ) {
    pixelRatio *= 0.75
  }
  return Math.max(0.25, pixelRatio)
}

/** Capture the full scrollable box of `el` (not just the visible viewport). */
export async function captureStructuredElement(el: HTMLElement): Promise<HTMLCanvasElement> {
  const backgroundColor = resolveThemeBackgroundColor()
  const width = Math.max(el.scrollWidth, el.offsetWidth, 1)
  const height = Math.max(el.scrollHeight, el.offsetHeight, 1)
  const pixelRatio = computePixelRatio(width, height)

  return toCanvas(el, {
    backgroundColor,
    width,
    height,
    canvasWidth: Math.ceil(width * pixelRatio),
    canvasHeight: Math.ceil(height * pixelRatio),
    pixelRatio,
    style: {
      overflow: 'visible',
      height: `${height}px`,
      maxHeight: 'none',
      width: `${width}px`,
    },
  })
}

function canvasToPngBlob(canvas: HTMLCanvasElement): Promise<Blob> {
  return new Promise((resolve, reject) => {
    canvas.toBlob((blob) => {
      if (blob) resolve(blob)
      else reject(new Error('PNG encoding failed'))
    }, 'image/png')
  })
}

/** Build a multi-page A4 PDF from a full-height canvas image (bitmap slices). */
export function canvasToPdfBlob(canvas: HTMLCanvasElement): Blob {
  const imgData = canvas.toDataURL('image/png')
  const pdf = new jsPDF({ orientation: 'portrait', unit: 'mm', format: 'a4' })
  const pageWidth = pdf.internal.pageSize.getWidth()
  const pageHeight = pdf.internal.pageSize.getHeight()
  const imgWidth = pageWidth
  const imgHeight = (canvas.height * imgWidth) / Math.max(canvas.width, 1)

  let heightLeft = imgHeight
  let position = 0

  pdf.addImage(imgData, 'PNG', 0, position, imgWidth, imgHeight)
  heightLeft -= pageHeight

  while (heightLeft > 0.5) {
    position -= pageHeight
    pdf.addPage()
    pdf.addImage(imgData, 'PNG', 0, position, imgWidth, imgHeight)
    heightLeft -= pageHeight
  }

  return pdf.output('blob')
}

/**
 * Export the visible StructuredArtifactView DOM to PNG or PDF.
 * Callers should surface incomplete/failure via toast.
 */
export async function exportStructuredArtifact(
  el: HTMLElement,
  artifactName: string,
  format: StructuredExportFormat,
  options?: { screenshotWaitMs?: number },
): Promise<StructuredExportResult> {
  const incomplete = !(await waitForTestScreenshots(el, options?.screenshotWaitMs ?? TEST_SCREENSHOT_WAIT_MS))
  const canvas = await captureStructuredElement(el)
  const filename = exportFilename(artifactName, format)

  if (format === 'png') {
    const blob = await canvasToPngBlob(canvas)
    triggerBlobDownload(filename, blob)
  } else {
    const blob = canvasToPdfBlob(canvas)
    triggerBlobDownload(filename, blob)
  }

  return { filename, incomplete }
}

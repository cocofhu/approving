// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  EXPORT_OUTER_PADDING_PX,
  canvasToPdfBlob,
  computePdfPageLayout,
  computePixelRatio,
  exportBasenameFromArtifactName,
  exportFilename,
  exportStructuredArtifact,
  resolveThemeBackgroundColor,
  waitForTestScreenshots,
} from './exportStructuredArtifact'

const toCanvasMock = vi.hoisted(() => vi.fn())

const jsPdfMocks = vi.hoisted(() => {
  const instances: Array<{
    addImage: ReturnType<typeof vi.fn>
    addPage: ReturnType<typeof vi.fn>
    output: ReturnType<typeof vi.fn>
    internal: { pageSize: { getWidth: () => number; getHeight: () => number } }
  }> = []
  class MockJsPDF {
    internal = { pageSize: { getWidth: () => 210, getHeight: () => 297 } }
    addImage = vi.fn()
    addPage = vi.fn()
    output = vi.fn(() => new Blob(['pdf'], { type: 'application/pdf' }))
    constructor() {
      instances.push(this)
    }
  }
  return { MockJsPDF, instances }
})

vi.mock('html-to-image', () => ({
  toCanvas: (...args: unknown[]) => toCanvasMock(...args),
}))

vi.mock('jspdf', () => ({
  jsPDF: jsPdfMocks.MockJsPDF,
}))

function mockCanvas(width = 100, height = 200): HTMLCanvasElement {
  const canvas = document.createElement('canvas')
  Object.defineProperty(canvas, 'width', { value: width })
  Object.defineProperty(canvas, 'height', { value: height })
  canvas.toDataURL = vi.fn(() => 'data:image/png;base64,xx')
  canvas.toBlob = vi.fn((cb: BlobCallback) => {
    cb(new Blob(['png'], { type: 'image/png' }))
  }) as typeof canvas.toBlob
  return canvas
}

describe('exportBasenameFromArtifactName / exportFilename', () => {
  it('strips last extension and appends png/pdf', () => {
    expect(exportBasenameFromArtifactName('clarified_requirement.json')).toBe('clarified_requirement')
    expect(exportFilename('clarified_requirement.json', 'png')).toBe('clarified_requirement.png')
    expect(exportFilename('clarified_requirement.json', 'pdf')).toBe('clarified_requirement.pdf')
  })

  it('sanitizes path-illegal characters without forcing .json', () => {
    expect(exportBasenameFromArtifactName('a/b\\c.json')).toBe('a_b_c')
    expect(exportFilename('plan.json', 'png')).toBe('plan.png')
  })

  it('falls back when name is empty or extension-only', () => {
    expect(exportBasenameFromArtifactName('')).toBe('artifact')
    expect(exportBasenameFromArtifactName('.json')).toBe('artifact')
  })
})

describe('resolveThemeBackgroundColor', () => {
  it('reads --c-base CSS variable as rgb()', () => {
    document.documentElement.style.setProperty('--c-base', '250 250 251')
    expect(resolveThemeBackgroundColor()).toBe('rgb(250 250 251)')
  })
})

describe('waitForTestScreenshots', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('resolves true immediately when no shimmer', async () => {
    const root = document.createElement('div')
    await expect(waitForTestScreenshots(root, 1000)).resolves.toBe(true)
  })

  it('resolves false after timeout while shimmer remains', async () => {
    const root = document.createElement('div')
    const shimmer = document.createElement('div')
    shimmer.className = 'shot-shimmer'
    root.appendChild(shimmer)
    const p = waitForTestScreenshots(root, 200)
    await vi.advanceTimersByTimeAsync(250)
    await expect(p).resolves.toBe(false)
  })

  it('resolves true when shimmer disappears before timeout', async () => {
    const root = document.createElement('div')
    const shimmer = document.createElement('div')
    shimmer.className = 'shot-shimmer'
    root.appendChild(shimmer)
    const p = waitForTestScreenshots(root, 1000)
    await vi.advanceTimersByTimeAsync(50)
    shimmer.remove()
    await vi.advanceTimersByTimeAsync(50)
    await expect(p).resolves.toBe(true)
  })
})

describe('exportStructuredArtifact', () => {
  beforeEach(() => {
    toCanvasMock.mockReset()
    toCanvasMock.mockResolvedValue(mockCanvas(400, 800))
    jsPdfMocks.instances.length = 0
    vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:export')
    vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {})
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockImplementation(((type: string) => {
      if (type !== '2d') return null
      return {
        fillStyle: '',
        fillRect: vi.fn(),
        drawImage: vi.fn(),
      }
    }) as unknown as typeof HTMLCanvasElement.prototype.getContext)
    vi.spyOn(HTMLCanvasElement.prototype, 'toDataURL').mockReturnValue('data:image/png;base64,page')
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('captures full size with theme background, 16px clone padding, and content+32 canvas (g3.1)', async () => {
    document.documentElement.style.setProperty('--c-base', '10 10 11')
    const root = document.createElement('div')
    Object.defineProperty(root, 'scrollWidth', { value: 640 })
    Object.defineProperty(root, 'scrollHeight', { value: 1200 })
    Object.defineProperty(root, 'offsetWidth', { value: 640 })
    Object.defineProperty(root, 'offsetHeight', { value: 400 })

    const result = await exportStructuredArtifact(root, 'research.json', 'png', { screenshotWaitMs: 0 })

    expect(result).toEqual({ filename: 'research.png', incomplete: false })
    expect(toCanvasMock).toHaveBeenCalledTimes(1)
    expect(EXPORT_OUTER_PADDING_PX).toBe(16)
    const opts = toCanvasMock.mock.calls[0][1] as {
      backgroundColor: string
      width: number
      height: number
      canvasWidth: number
      canvasHeight: number
      pixelRatio: number
      style: Record<string, string>
    }
    const contentW = 640
    const contentH = 1200
    const exportW = contentW + 2 * EXPORT_OUTER_PADDING_PX
    const exportH = contentH + 2 * EXPORT_OUTER_PADDING_PX
    const pixelRatio = computePixelRatio(exportW, exportH)
    expect(opts.backgroundColor).toBe('rgb(10 10 11)')
    expect(opts.width).toBe(exportW)
    expect(opts.height).toBe(exportH)
    expect(opts.style.padding).toBe('16px')
    expect(opts.style.boxSizing).toBe('border-box')
    expect(opts.style.width).toBe(`${exportW}px`)
    expect(opts.style.height).toBe(`${exportH}px`)
    expect(opts.pixelRatio).toBe(pixelRatio)
    expect(opts.canvasWidth).toBe(Math.ceil(exportW * pixelRatio))
    expect(opts.canvasHeight).toBe(Math.ceil(exportH * pixelRatio))
    // g1.1: html-to-image 1.11.13 has no onclone — options must not pass one
    expect(opts).not.toHaveProperty('onclone')
  })

  it('downloads pdf filename for pdf format', async () => {
    const root = document.createElement('div')
    const result = await exportStructuredArtifact(root, 'plan.json', 'pdf', { screenshotWaitMs: 0 })
    expect(result.filename).toBe('plan.pdf')
    expect(result.incomplete).toBe(false)
  })

  it('marks incomplete when screenshots still loading after wait', async () => {
    vi.useFakeTimers()
    const root = document.createElement('div')
    const shimmer = document.createElement('div')
    shimmer.className = 'shot-shimmer'
    root.appendChild(shimmer)

    const p = exportStructuredArtifact(root, 'test_result.json', 'png', { screenshotWaitMs: 100 })
    await vi.advanceTimersByTimeAsync(150)
    const result = await p
    expect(result.incomplete).toBe(true)
    expect(result.filename).toBe('test_result.png')
    vi.useRealTimers()
  })
})

describe('computePdfPageLayout / canvasToPdfBlob (g3.2)', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    jsPdfMocks.instances.length = 0
  })

  it('slices content region only so every page has inset destY (not whole-canvas negative Y)', () => {
    const canvas = { width: 672, height: 3000 }
    const layout = computePdfPageLayout(canvas, { exportCssWidth: 672 })
    expect(layout.marginMm).toBeCloseTo((EXPORT_OUTER_PADDING_PX / 672) * 210)
    expect(layout.insetPx).toBeCloseTo(EXPORT_OUTER_PADDING_PX)
    expect(layout.slices.length).toBeGreaterThan(1)
    for (const slice of layout.slices) {
      expect(slice.destX).toBeCloseTo(layout.insetPx)
      expect(slice.destY).toBeCloseTo(layout.insetPx)
      expect(slice.srcX).toBeCloseTo(layout.insetPx)
      expect(slice.srcY).toBeGreaterThanOrEqual(layout.insetPx - 1e-6)
      expect(slice.srcY + slice.srcH).toBeLessThanOrEqual(3000 - layout.insetPx + 1e-6)
    }
    expect(layout.slices[0]!.srcY).toBeCloseTo(layout.insetPx)
    expect(layout.slices[1]!.destY).toBeCloseTo(layout.insetPx)
  })

  it('composites theme fill + inset drawImage on every page of a tall canvas', () => {
    jsPdfMocks.instances.length = 0
    const fillRect = vi.fn()
    const drawImage = vi.fn()
    const fillStyles: string[] = []
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockImplementation(((type: string) => {
      if (type !== '2d') return null
      return {
        get fillStyle() {
          return fillStyles[fillStyles.length - 1] ?? ''
        },
        set fillStyle(value: string) {
          fillStyles.push(value)
        },
        fillRect,
        drawImage,
      }
    }) as unknown as typeof HTMLCanvasElement.prototype.getContext)
    vi.spyOn(HTMLCanvasElement.prototype, 'toDataURL').mockReturnValue('data:image/png;base64,page')

    const canvas = mockCanvas(672, 3000)
    const blob = canvasToPdfBlob(canvas, {
      exportCssWidth: 672,
      backgroundColor: 'rgb(10 10 11)',
    })
    expect(blob.type).toBe('application/pdf')

    const layout = computePdfPageLayout(canvas, { exportCssWidth: 672 })
    expect(layout.slices.length).toBeGreaterThan(1)
    expect(jsPdfMocks.instances).toHaveLength(1)
    const pdf = jsPdfMocks.instances[0]!
    expect(pdf.addPage).toHaveBeenCalledTimes(layout.slices.length - 1)
    expect(pdf.addImage).toHaveBeenCalledTimes(layout.slices.length)
    for (const call of pdf.addImage.mock.calls) {
      expect(call[2]).toBe(0)
      expect(call[3]).toBe(0)
      expect(call[4]).toBe(210)
      expect(call[5]).toBe(297)
      expect(call[3]).toBeGreaterThanOrEqual(0)
    }
    expect(fillRect).toHaveBeenCalledTimes(layout.slices.length)
    expect(drawImage).toHaveBeenCalledTimes(layout.slices.length)
    expect(fillStyles.every((c) => c === 'rgb(10, 10, 11)')).toBe(true)
    for (let i = 0; i < layout.slices.length; i++) {
      const slice = layout.slices[i]!
      expect(fillRect.mock.calls[i]!.slice(0, 4)).toEqual([
        0,
        0,
        layout.pageBmpWidth,
        layout.pageBmpHeight,
      ])
      const drawArgs = drawImage.mock.calls[i]!
      expect(drawArgs[0]).toBe(canvas)
      expect(drawArgs[1]).toBeCloseTo(slice.srcX)
      expect(drawArgs[2]).toBeCloseTo(slice.srcY)
      expect(drawArgs[3]).toBeCloseTo(slice.srcW)
      expect(drawArgs[4]).toBeCloseTo(slice.srcH)
      expect(drawArgs[5]).toBeCloseTo(slice.destX)
      expect(drawArgs[6]).toBeCloseTo(slice.destY)
      expect(drawArgs[7]).toBeCloseTo(slice.destW)
      expect(drawArgs[8]).toBeCloseTo(slice.destH)
      expect(drawArgs[6]).toBeGreaterThan(0)
    }
  })
})

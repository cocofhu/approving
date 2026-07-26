// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  exportBasenameFromArtifactName,
  exportFilename,
  exportStructuredArtifact,
  resolveThemeBackgroundColor,
  waitForTestScreenshots,
} from './exportStructuredArtifact'

const toCanvasMock = vi.hoisted(() => vi.fn())

vi.mock('html-to-image', () => ({
  toCanvas: (...args: unknown[]) => toCanvasMock(...args),
}))

vi.mock('jspdf', () => {
  class MockJsPDF {
    internal = { pageSize: { getWidth: () => 210, getHeight: () => 297 } }
    addImage = vi.fn()
    addPage = vi.fn()
    output = vi.fn(() => new Blob(['pdf'], { type: 'application/pdf' }))
  }
  return { jsPDF: MockJsPDF }
})

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
    vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:export')
    vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {})
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('captures full size with theme background and downloads png', async () => {
    document.documentElement.style.setProperty('--c-base', '10 10 11')
    const root = document.createElement('div')
    Object.defineProperty(root, 'scrollWidth', { value: 640 })
    Object.defineProperty(root, 'scrollHeight', { value: 1200 })
    Object.defineProperty(root, 'offsetWidth', { value: 640 })
    Object.defineProperty(root, 'offsetHeight', { value: 400 })

    const result = await exportStructuredArtifact(root, 'research.json', 'png', { screenshotWaitMs: 0 })

    expect(result).toEqual({ filename: 'research.png', incomplete: false })
    expect(toCanvasMock).toHaveBeenCalledTimes(1)
    const opts = toCanvasMock.mock.calls[0][1]
    expect(opts.backgroundColor).toBe('rgb(10 10 11)')
    expect(opts.width).toBe(640)
    expect(opts.height).toBe(1200)
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

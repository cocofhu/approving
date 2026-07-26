// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { copyToClipboard } from './copyToClipboard'

describe('copyToClipboard', () => {
  let originalIsSecureContext: PropertyDescriptor | undefined
  let originalClipboard: PropertyDescriptor | undefined
  let execCommand: ReturnType<typeof vi.fn>

  beforeEach(() => {
    originalIsSecureContext = Object.getOwnPropertyDescriptor(window, 'isSecureContext')
    originalClipboard = Object.getOwnPropertyDescriptor(navigator, 'clipboard')
    execCommand = vi.fn().mockReturnValue(true)
    Object.defineProperty(document, 'execCommand', {
      configurable: true,
      writable: true,
      value: execCommand,
    })
  })

  afterEach(() => {
    if (originalIsSecureContext) {
      Object.defineProperty(window, 'isSecureContext', originalIsSecureContext)
    }
    if (originalClipboard) {
      Object.defineProperty(navigator, 'clipboard', originalClipboard)
    } else {
      // @ts-expect-error restore undefined clipboard in happy-dom
      delete navigator.clipboard
    }
    vi.restoreAllMocks()
  })

  function stubSecureContext(value: boolean) {
    Object.defineProperty(window, 'isSecureContext', {
      configurable: true,
      get: () => value,
    })
  }

  function stubClipboard(writeText: ((text: string) => Promise<void>) | undefined) {
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: writeText ? { writeText } : undefined,
    })
  }

  it('secure context: writeText resolves → true and does not call execCommand', async () => {
    stubSecureContext(true)
    const writeText = vi.fn().mockResolvedValue(undefined)
    stubClipboard(writeText)

    await expect(copyToClipboard('hello')).resolves.toBe(true)
    expect(writeText).toHaveBeenCalledWith('hello')
    expect(execCommand).not.toHaveBeenCalled()
  })

  it('non-secure context: skips API and uses legacy copy', async () => {
    stubSecureContext(false)
    const writeText = vi.fn().mockResolvedValue(undefined)
    stubClipboard(writeText)
    execCommand.mockReturnValue(true)

    await expect(copyToClipboard('192.168.2.182:8765')).resolves.toBe(true)
    expect(writeText).not.toHaveBeenCalled()
    expect(execCommand).toHaveBeenCalledWith('copy')
  })

  it('missing clipboard: falls back to execCommand', async () => {
    stubSecureContext(true)
    stubClipboard(undefined)
    execCommand.mockReturnValue(true)

    await expect(copyToClipboard('/sandbox/proxy/80/')).resolves.toBe(true)
    expect(execCommand).toHaveBeenCalledWith('copy')
  })

  it('secure context: writeText rejects then legacy succeeds → true', async () => {
    stubSecureContext(true)
    const writeText = vi.fn().mockRejectedValue(new Error('denied'))
    stubClipboard(writeText)
    execCommand.mockReturnValue(true)

    await expect(copyToClipboard('fallback-ok')).resolves.toBe(true)
    expect(writeText).toHaveBeenCalled()
    expect(execCommand).toHaveBeenCalledWith('copy')
  })

  it('both paths fail → false', async () => {
    stubSecureContext(true)
    const writeText = vi.fn().mockRejectedValue(new Error('denied'))
    stubClipboard(writeText)
    execCommand.mockReturnValue(false)

    await expect(copyToClipboard('nope')).resolves.toBe(false)
    expect(writeText).toHaveBeenCalled()
    expect(execCommand).toHaveBeenCalledWith('copy')
  })

  it('non-secure context and execCommand fails → false', async () => {
    stubSecureContext(false)
    stubClipboard(undefined)
    execCommand.mockReturnValue(false)

    await expect(copyToClipboard('nope')).resolves.toBe(false)
    expect(execCommand).toHaveBeenCalledWith('copy')
  })
})

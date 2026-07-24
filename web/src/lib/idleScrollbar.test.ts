// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  IDLE_SCROLLBAR_HIDE_MS,
  installIdleScrollbar,
  isIdleScrollbarInstalled,
  uninstallIdleScrollbar,
} from './idleScrollbar'

describe('idleScrollbar', () => {
  beforeEach(() => {
    uninstallIdleScrollbar()
    document.body.innerHTML = ''
    document.body.className = ''
    document.documentElement.className = ''
    vi.useFakeTimers()
  })

  afterEach(() => {
    uninstallIdleScrollbar()
    document.body.className = ''
    document.documentElement.className = ''
    vi.useRealTimers()
  })

  it('installs once and can uninstall', () => {
    installIdleScrollbar()
    expect(isIdleScrollbarInstalled()).toBe(true)
    installIdleScrollbar()
    expect(isIdleScrollbarInstalled()).toBe(true)
    uninstallIdleScrollbar()
    expect(isIdleScrollbarInstalled()).toBe(false)
  })

  it('uninstall clears pending is-scrolling and hide timers', () => {
    installIdleScrollbar()
    const box = document.createElement('div')
    document.body.appendChild(box)

    box.dispatchEvent(new Event('scroll', { bubbles: true }))
    expect(box.classList.contains('is-scrolling')).toBe(true)

    uninstallIdleScrollbar()
    expect(box.classList.contains('is-scrolling')).toBe(false)

    vi.advanceTimersByTime(IDLE_SCROLLBAR_HIDE_MS)
    expect(box.classList.contains('is-scrolling')).toBe(false)
  })

  it('adds is-scrolling on scroll and clears after ~800ms', () => {
    installIdleScrollbar()
    const box = document.createElement('div')
    box.className = 'scroll-area'
    document.body.appendChild(box)

    box.dispatchEvent(new Event('scroll', { bubbles: true }))
    expect(box.classList.contains('is-scrolling')).toBe(true)

    vi.advanceTimersByTime(IDLE_SCROLLBAR_HIDE_MS - 1)
    expect(box.classList.contains('is-scrolling')).toBe(true)

    vi.advanceTimersByTime(1)
    expect(box.classList.contains('is-scrolling')).toBe(false)
  })

  it('resets hide timer on continuous scroll', () => {
    installIdleScrollbar()
    const box = document.createElement('div')
    document.body.appendChild(box)

    box.dispatchEvent(new Event('scroll', { bubbles: true }))
    vi.advanceTimersByTime(500)
    box.dispatchEvent(new Event('scroll', { bubbles: true }))
    vi.advanceTimersByTime(500)
    expect(box.classList.contains('is-scrolling')).toBe(true)
    vi.advanceTimersByTime(IDLE_SCROLLBAR_HIDE_MS)
    expect(box.classList.contains('is-scrolling')).toBe(false)
  })

  it('ignores Monaco editor scroll targets', () => {
    installIdleScrollbar()
    const host = document.createElement('div')
    host.className = 'monaco-editor'
    const inner = document.createElement('div')
    host.appendChild(inner)
    document.body.appendChild(host)

    inner.dispatchEvent(new Event('scroll', { bubbles: true }))
    expect(inner.classList.contains('is-scrolling')).toBe(false)
    expect(host.classList.contains('is-scrolling')).toBe(false)
  })

  it('maps document / html / body scroll targets to documentElement or body', () => {
    installIdleScrollbar()

    document.dispatchEvent(new Event('scroll', { bubbles: true }))
    expect(document.documentElement.classList.contains('is-scrolling')).toBe(true)

    document.documentElement.classList.remove('is-scrolling')
    document.documentElement.dispatchEvent(new Event('scroll', { bubbles: true }))
    expect(document.documentElement.classList.contains('is-scrolling')).toBe(true)

    document.body.dispatchEvent(new Event('scroll', { bubbles: true }))
    expect(document.body.classList.contains('is-scrolling')).toBe(true)
  })
})

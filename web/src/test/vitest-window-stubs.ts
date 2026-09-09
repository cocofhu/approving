// Vitest 4 + happy-dom: window.confirm/alert/prompt may be undefined, so
// vi.spyOn(window, 'confirm') throws. Install writable stubs before tests.
if (typeof window !== 'undefined') {
  const stub = (name: 'confirm' | 'alert' | 'prompt', impl: (...args: unknown[]) => unknown) => {
    if (typeof window[name] === 'function') return
    Object.defineProperty(window, name, {
      configurable: true,
      writable: true,
      value: impl,
    })
  }
  stub('confirm', () => true)
  stub('alert', () => undefined)
  stub('prompt', () => null)
}

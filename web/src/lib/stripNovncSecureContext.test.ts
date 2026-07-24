import { describe, it, expect } from 'vitest'
import {
  stripNovncSecureContextCheck,
  SECURE_CONTEXT_SNIPPET,
} from '../../vite-plugins/stripNovncSecureContext'

describe('stripNovncSecureContextCheck', () => {
  it('removes the isSecureContext Log.Error block from noVNC rfb.js shape', () => {
    const sample = `
    if (!urlOrChannel) {
      throw new Error("Must specify URL, WebSocket or RTCDataChannel");
    }

    // We rely on modern APIs which might not be available in an
    // insecure context
    if (!window.isSecureContext) {
      Log.Error("noVNC requires a secure context (TLS). Expect crashes!");
    }
    _this = _callSuper(this, RFB);
`
    const out = stripNovncSecureContextCheck(sample)
    expect(out).not.toBeNull()
    expect(out!).not.toContain(SECURE_CONTEXT_SNIPPET)
    expect(out!).not.toContain('isSecureContext')
    expect(out!).toContain('_this = _callSuper(this, RFB)')
  })

  it('returns null when the snippet is absent', () => {
    expect(stripNovncSecureContextCheck('const x = 1')).toBeNull()
  })
})

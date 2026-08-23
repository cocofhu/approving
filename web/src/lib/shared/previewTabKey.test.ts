import { describe, expect, it } from 'vitest'
import { isUrlPreview, previewTabKey, previewTabLabel } from './previewTabKey'
import type { PreviewPort } from '@/lib/api/apiTypes'

describe('previewTabKey', () => {
  it('distinguishes url and port tabs', () => {
    const port: PreviewPort = {
      runId: 'r',
      nodeId: 'n',
      kind: 'port',
      port: 5173,
      proxyUrl: '/preview/r/n/5173/',
      healthy: true,
    }
    const url: PreviewPort = {
      runId: 'r',
      nodeId: 'n',
      kind: 'url',
      port: 0,
      url: 'https://staging.example.com:8443/',
      proxyUrl: '',
      healthy: true,
      label: 'Staging',
    }
    expect(previewTabKey(port)).toBe('port:5173')
    expect(previewTabKey(url)).toBe('url:https://staging.example.com:8443/')
    expect(isUrlPreview(url)).toBe(true)
    expect(isUrlPreview(port)).toBe(false)
    expect(previewTabLabel(url)).toBe('Staging')
    expect(previewTabLabel({ ...port, label: '' })).toBe('5173')
  })
})

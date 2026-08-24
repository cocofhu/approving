import { describe, expect, it } from 'vitest'
import { isNodeEventsUnavailable } from './nodeEventsResponse'

describe('nodeEventsResponse', () => {
  it('isNodeEventsUnavailable is true only when unavailable flag is set', () => {
    expect(isNodeEventsUnavailable({ events: [], live: false, unavailable: true })).toBe(true)
    expect(isNodeEventsUnavailable({ events: [], live: false })).toBe(false)
    expect(isNodeEventsUnavailable({ events: [], live: false, error: 'x' })).toBe(false)
    expect(isNodeEventsUnavailable(null)).toBe(false)
    expect(isNodeEventsUnavailable(undefined)).toBe(false)
  })
})

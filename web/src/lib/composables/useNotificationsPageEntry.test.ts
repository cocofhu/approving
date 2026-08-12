import { describe, expect, it } from 'vitest'
import {
  __resetNotificationsPageEntryForTests,
  requestNotificationsPageReset,
  useNotificationsPageEntry,
} from './useNotificationsPageEntry'

describe('useNotificationsPageEntry', () => {
  it('increments a shared nonce so same-route view-all can reset (g3.1)', () => {
    __resetNotificationsPageEntryForTests()
    const a = useNotificationsPageEntry()
    const b = useNotificationsPageEntry()
    expect(a.enterNonce.value).toBe(0)
    requestNotificationsPageReset()
    expect(a.enterNonce.value).toBe(1)
    expect(b.enterNonce.value).toBe(1)
    __resetNotificationsPageEntryForTests()
    expect(a.enterNonce.value).toBe(0)
  })
})

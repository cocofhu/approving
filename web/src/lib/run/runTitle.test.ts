import { describe, expect, it } from 'vitest'
import { displayRunTitle } from './runTitle'

describe('displayRunTitle', () => {
  it('keeps ordinary titles', () => {
    expect(displayRunTitle('邮箱验证码登录')).toBe('邮箱验证码登录')
  })

  it('formats a repos JSON dump as repo names', () => {
    expect(
      displayRunTitle('[{"branch":"","name":"approving","url":"https://git.example/approving.git"}]'),
    ).toBe('approving')
    expect(
      displayRunTitle(
        '[{"name":"web","url":"https://h/w.git"},{"name":"api","url":"https://h/a.git"}]',
      ),
    ).toBe('web · api')
  })

  it('hides other raw JSON', () => {
    expect(displayRunTitle('{"foo":1}')).toBe('')
    expect(displayRunTitle('')).toBe('')
    expect(displayRunTitle(null)).toBe('')
  })
})

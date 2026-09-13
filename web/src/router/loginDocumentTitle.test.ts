// @vitest-environment happy-dom
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const here = dirname(fileURLToPath(import.meta.url))
const routerSrc = readFileSync(join(here, 'index.ts'), 'utf8')
const zhRoute = JSON.parse(readFileSync(join(here, '../locales/zh-CN/route.json'), 'utf8'))
const enRoute = JSON.parse(readFileSync(join(here, '../locales/en/route.json'), 'utf8'))
const zhShell = JSON.parse(readFileSync(join(here, '../locales/zh-CN/shell.json'), 'utf8'))
const enShell = JSON.parse(readFileSync(join(here, '../locales/en/shell.json'), 'utf8'))

describe('login route titleKey + existing copy (plan g2.2)', () => {
  it('keeps /login on route.login without renaming keys', () => {
    expect(routerSrc).toMatch(
      /path:\s*'\/login'[\s\S]*?meta:\s*\{\s*titleKey:\s*'route\.login'/,
    )
  })

  it('zh-CN and en login titles compose to 登录/Login · Grasp', () => {
    expect(zhRoute.route.login).toBe('登录')
    expect(enRoute.route.login).toBe('Login')
    expect(zhShell.shell.appName).toBe('Grasp')
    expect(enShell.shell.appName).toBe('Grasp')
    expect(`${zhRoute.route.login} · ${zhShell.shell.appName}`).toBe('登录 · Grasp')
    expect(`${enRoute.route.login} · ${enShell.shell.appName}`).toBe('Login · Grasp')
    expect(zhRoute.route.runs).toBe('运行记录')
    expect(enRoute.route.runs).toBe('Run history')
  })
})

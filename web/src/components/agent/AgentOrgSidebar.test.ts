// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import enCommon from '@/locales/en/common.json'
import enPages from '@/locales/en/pages.json'
import type { AgentOrg } from '@/lib/api'
import AgentOrgSidebar from './AgentOrgSidebar.vue'

const sampleOrg: AgentOrg = {
  revision: 1,
  groups: [
    { id: 'g_dev', name: '开发部门', parentGroupId: '' },
    { id: 'g_sub', name: '前端组', parentGroupId: 'g_dev' },
    { id: 'g_qa', name: '测试部门', parentGroupId: '' },
  ],
  agents: {
    alice: { groupIds: ['g_dev'] },
    bob: { groupIds: ['g_sub'], parentAgent: 'alice' },
    // 10 members on g_qa → two-digit count
    ...Object.fromEntries(
      Array.from({ length: 10 }, (_, i) => [`qa${i}`, { groupIds: ['g_qa'] }]),
    ),
  },
}

const agentNames = ['alice', 'bob', ...Array.from({ length: 10 }, (_, i) => `qa${i}`), 'orphan']

function mountSidebar(org: AgentOrg = sampleOrg, teleport = false) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(AgentOrgSidebar, {
    props: {
      org,
      agentNames,
      activeName: '',
      collapsed: false,
    },
    attachTo: teleport ? document.body : undefined,
    global: {
      plugins: [i18n],
      stubs: {
        Icon: { template: '<span class="icon" />' },
        Teleport: teleport ? false : true,
      },
    },
  })
}

/** Match agent leaf by exact name span (avoids reportsTo text false positives). */
function findAgentRow(wrapper: ReturnType<typeof mountSidebar>, name: string) {
  return wrapper.findAll('[data-org-kind="agent"]').find((r) => {
    const nameEl = r.find('.truncate.text-txt')
    return nameEl.exists() && nameEl.text() === name
  })
}

describe('AgentOrgSidebar two-column right-edge count layout', () => {
  it('组行仅为 main + count 两列，无 hover 操作列，徽章贴右', () => {
    const wrapper = mountSidebar()
    const groupRow = wrapper.find('[data-org-kind="group"]')
    expect(groupRow.exists()).toBe(true)

    const children = groupRow.element.children
    expect(children).toHaveLength(2)
    expect(children[0].hasAttribute('data-org-main')).toBe(true)
    expect(children[1].hasAttribute('data-org-count')).toBe(true)
    expect(groupRow.find('[data-org-actions]').exists()).toBe(false)

    const count = children[1]
    expect(count.className).toContain('justify-end')
    expect(count.className).toContain('w-auto')
    expect(count.className).toContain('shrink-0')
    expect(count.className).not.toContain('justify-start')
    expect(count.className).not.toContain('w-7')
    expect(groupRow.find('[data-org-main]').html()).not.toContain('data-org-count')

    const badge = count.querySelector('span')
    expect(badge?.className).toContain('justify-end')
    expect(badge?.className).toContain('min-w-[18px]')
    expect(badge?.className).toContain('tabular-nums')
  })

  it('未分组头行两列贴右计数，无操作占位，右键不弹组菜单', async () => {
    const wrapper = mountSidebar(sampleOrg, true)
    const ug = wrapper.find('[data-org-kind="ungrouped-header"]')
    expect(ug.exists()).toBe(true)

    const children = ug.element.children
    expect(children).toHaveLength(2)
    expect(children[0].hasAttribute('data-org-main')).toBe(true)
    expect(children[1].hasAttribute('data-org-count')).toBe(true)
    expect(ug.find('[data-org-actions-spacer]').exists()).toBe(false)
    expect(children[1].className).toContain('justify-end')

    await ug.trigger('contextmenu', { clientX: 40, clientY: 50 })
    expect(document.querySelector('[data-org-ctx-menu]')).toBeNull()
    wrapper.unmount()
  })

  it('depth 缩进只作用在左侧 main，count 列不受 paddingLeft', () => {
    const wrapper = mountSidebar()
    const nested = wrapper
      .findAll('[data-org-kind="group"]')
      .find((r) => r.attributes('data-org-depth') === '1')
    expect(nested).toBeTruthy()

    const main = nested!.find('[data-org-main]')
    const count = nested!.find('[data-org-count]')
    expect(main.attributes('style') || '').toMatch(/padding-left:\s*20px/)
    expect(count.attributes('style') || '').not.toMatch(/padding-left/)
    expect((nested!.element as HTMLElement).getAttribute('style') || '').not.toMatch(/padding-left/)
  })

  it('根组、嵌套子组与未分组计数右齐且支持三位数左扩', () => {
    const orgWithTriple: AgentOrg = {
      ...sampleOrg,
      agents: {
        ...sampleOrg.agents,
        ...Object.fromEntries(
          Array.from({ length: 118 }, (_, i) => [`bulk${i}`, { groupIds: ['g_dev'] }]),
        ),
      },
    }
    const names = [
      ...agentNames,
      ...Array.from({ length: 118 }, (_, i) => `bulk${i}`),
    ]
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const wrapper = mount(AgentOrgSidebar, {
      props: {
        org: orgWithTriple,
        agentNames: names,
        activeName: '',
        collapsed: false,
      },
      global: {
        plugins: [i18n],
        stubs: {
          Icon: { template: '<span class="icon" />' },
          Teleport: true,
        },
      },
    })

    const counts = wrapper.findAll('[data-org-count]')
    expect(counts.length).toBeGreaterThanOrEqual(3)
    for (const c of counts) {
      expect(c.classes()).toContain('justify-end')
      expect(c.classes()).toContain('shrink-0')
      expect(c.classes()).toContain('w-auto')
      expect(c.classes()).not.toContain('w-7')
    }

    const qaCount = wrapper
      .findAll('[data-org-kind="group"]')
      .find((r) => r.text().includes('测试部门'))
      ?.find('[data-org-count]')
    expect(qaCount?.text()).toBe('10')

    const oneDigit = wrapper
      .findAll('[data-org-kind="group"]')
      .find((r) => r.text().includes('前端组'))
      ?.find('[data-org-count]')
    expect(oneDigit?.text()).toBe('1')

    const triple = wrapper
      .findAll('[data-org-kind="group"]')
      .find((r) => r.text().includes('开发部门'))
      ?.find('[data-org-count]')
    // alice + 118 bulk = 119
    expect(triple?.text()).toBe('119')
    expect(triple?.classes()).toContain('w-auto')
  })

  it('折叠入口仍可用，且无 hover 操作按钮', async () => {
    const wrapper = mountSidebar()
    const group = wrapper.findAll('[data-org-kind="group"]')[0]
    expect(group.find('[data-org-actions]').exists()).toBe(false)
    expect(group.findAll('button').filter((b) => b.attributes('data-org-toggle') !== undefined).length).toBe(1)

    await group.find('[data-org-toggle]').trigger('click')
    expect(wrapper.findAll('[data-org-kind="group"]').some((r) => r.text().includes('前端组'))).toBe(
      false,
    )
  })

  it('叶子行无计数徽章、无 hover 操作，仍可选中/拖拽', async () => {
    const wrapper = mountSidebar()
    const leaf = wrapper.find('[data-org-kind="agent"]')
    expect(leaf.exists()).toBe(true)
    expect(leaf.find('[data-org-count]').exists()).toBe(false)
    expect(leaf.find('[data-org-count-spacer]').exists()).toBe(false)
    expect(leaf.findAll('button').length).toBe(1)

    const dragBtn = leaf.find('button[draggable="true"]')
    expect(dragBtn.exists()).toBe(true)
    await dragBtn.trigger('click')
    expect(wrapper.emitted('select-agent')?.[0]).toEqual(['bob'])
  })

  it('拖拽 agent 到未分组头可发出 move-agent', async () => {
    const wrapper = mountSidebar()
    const leaf = wrapper.find('[data-org-kind="agent"]')
    const dragBtn = leaf.find('button[draggable="true"]')
    await dragBtn.trigger('dragstart', {
      dataTransfer: { setData: () => {}, effectAllowed: 'move' },
    })
    const ug = wrapper.find('[data-org-kind="ungrouped-header"]')
    await ug.trigger('dragover', { preventDefault: () => {} })
    await ug.trigger('drop', { preventDefault: () => {} })
    expect(wrapper.emitted('move-agent')?.[0]).toEqual(['bob', 'g_sub', ''])
  })

  it('侧栏不出现 L1/L2 层级标签', () => {
    const wrapper = mountSidebar()
    expect(wrapper.text()).not.toMatch(/\bL[12]\b/)
  })
})

describe('AgentOrgSidebar context menus', () => {
  it('组头行右键弹出新建子组/重命名/删除', async () => {
    const wrapper = mountSidebar(sampleOrg, true)
    const group = wrapper
      .findAll('[data-org-kind="group"]')
      .find((r) => r.text().includes('开发部门'))!
    await group.trigger('contextmenu', { clientX: 12, clientY: 24 })

    const menu = document.querySelector('[data-org-ctx-menu]') as HTMLElement | null
    expect(menu).toBeTruthy()
    expect(menu?.getAttribute('data-org-ctx-kind')).toBe('group')
    expect(menu?.querySelectorAll('[data-org-ctx-action]')).toHaveLength(3)
    expect(menu?.textContent).toContain('新建子组')

    const renameBtn = menu!.querySelector('[data-org-ctx-action="rename"]') as HTMLButtonElement
    renameBtn.click()
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('rename-group')?.[0]).toEqual(['g_dev'])
    expect(document.querySelector('[data-org-ctx-menu]')).toBeNull()
    wrapper.unmount()
  })

  it('已分组 Agent 菜单含重命名直达管理与移出本组', async () => {
    const wrapper = mountSidebar(sampleOrg, true)
    const leaf = findAgentRow(wrapper, 'bob')!
    await leaf.trigger('contextmenu', { clientX: 20, clientY: 30 })

    const menu = document.querySelector('[data-org-ctx-menu]') as HTMLElement | null
    expect(menu?.getAttribute('data-org-ctx-kind')).toBe('agent')
    expect(menu?.querySelector('[data-org-ctx-action="renameViaManage"]')).toBeTruthy()
    expect(menu?.querySelector('[data-org-ctx-action="removeFromGroup"]')).toBeTruthy()
    expect(menu?.textContent).toContain('重命名（前往管理）')
    expect(menu?.textContent).toContain('移出本组')

    const renameBtn = menu!.querySelector(
      '[data-org-ctx-action="renameViaManage"]',
    ) as HTMLButtonElement
    renameBtn.click()
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('open-manage')?.[0]).toEqual(['bob'])
    expect(wrapper.emitted('rename-agent')).toBeUndefined()
    wrapper.unmount()
  })

  it('未分组 Agent 菜单仅重命名（前往管理）', async () => {
    const wrapper = mountSidebar(sampleOrg, true)
    const leaf = findAgentRow(wrapper, 'orphan')!
    expect(leaf).toBeTruthy()
    await leaf.trigger('contextmenu', { clientX: 20, clientY: 30 })

    const menu = document.querySelector('[data-org-ctx-menu]') as HTMLElement | null
    expect(menu?.querySelector('[data-org-ctx-action="renameViaManage"]')).toBeTruthy()
    expect(menu?.querySelector('[data-org-ctx-action="removeFromGroup"]')).toBeNull()
    expect(menu?.textContent).toContain('重命名（前往管理）')
    expect(menu?.textContent).not.toContain('移出本组')
    wrapper.unmount()
  })

  it('移出本组发出 remove-from-group', async () => {
    const wrapper = mountSidebar(sampleOrg, true)
    const leaf = findAgentRow(wrapper, 'alice')!
    await leaf.trigger('contextmenu', { clientX: 20, clientY: 30 })
    const removeBtn = document.querySelector(
      '[data-org-ctx-action="removeFromGroup"]',
    ) as HTMLButtonElement
    removeBtn.click()
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('remove-from-group')?.[0]).toEqual(['alice', 'g_dev'])
    wrapper.unmount()
  })
})

describe('AgentOrgSidebar dragHint removal', () => {
  function mountSidebarEn(org: AgentOrg = sampleOrg) {
    const i18n = createI18n({
      legacy: false,
      locale: 'en',
      messages: { en: { ...enCommon, ...enPages } },
    })
    return mount(AgentOrgSidebar, {
      props: {
        org,
        agentNames,
        activeName: '',
        collapsed: false,
      },
      global: {
        plugins: [i18n],
        stubs: {
          Icon: { template: '<span class="icon" />' },
          Teleport: true,
        },
      },
    })
  }

  it('不再展示中文 dragHint 与虚线提示框', () => {
    const wrapper = mountSidebar()
    expect(wrapper.text()).not.toContain('拖拽 Agent 调整归属')
    expect(wrapper.text()).not.toContain('真删请用顶栏 Agent 管理入口')
    expect(wrapper.find('.scroll-area p.border-dashed').exists()).toBe(false)
    expect(wrapper.html()).not.toContain('border-dashed border-line-strong')
    expect(wrapper.find('[data-org-manage]').exists()).toBe(true)
  })

  it('不再展示英文 dragHint', () => {
    const wrapper = mountSidebarEn()
    expect(wrapper.text()).not.toContain('Drag agents to change membership')
    expect(wrapper.text()).not.toContain('hard delete via the Agent management')
    expect(wrapper.find('.scroll-area p.border-dashed').exists()).toBe(false)
    expect(wrapper.text()).toContain('Ungrouped')
  })
})

describe('AgentOrgSidebar manage icon-only', () => {
  it('管理入口仅图标，无「管理」可见文案，保留 title/aria-label', () => {
    const wrapper = mountSidebar()
    const manage = wrapper.find('[data-org-manage]')
    expect(manage.exists()).toBe(true)
    expect(manage.text().trim()).toBe('')
    expect(manage.attributes('title')).toBe('Agent 管理')
    expect(manage.attributes('aria-label')).toBe('Agent 管理')
    expect(manage.classes()).toContain('w-[22px]')
    // 按钮可见区域不含「管理」二字（文案仅在 title/aria-label）
    expect(manage.element.textContent?.includes('管理')).toBe(false)
  })
})

describe('AgentOrgSidebar agent name text color', () => {
  function agentNameClass(wrapper: ReturnType<typeof mountSidebar>, agentName: string) {
    const leaf = findAgentRow(wrapper, agentName)!
    const nameSpan = leaf.find('.truncate.text-txt')
    expect(nameSpan.exists()).toBe(true)
    expect(nameSpan.text()).toBe(agentName)
    return nameSpan.classes().join(' ')
  }

  it('选中与未选中 Agent 名称均含 text-txt、不含灰色 utility，选中行有 accent-dim', () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const wrapper = mount(AgentOrgSidebar, {
      props: {
        org: sampleOrg,
        agentNames,
        activeName: 'bob',
        collapsed: false,
      },
      global: {
        plugins: [i18n],
        stubs: {
          Icon: { template: '<span class="icon" />' },
          Teleport: true,
        },
      },
    })

    const selected = findAgentRow(wrapper, 'bob')!
    expect(selected.classes()).toContain('bg-accent-dim')
    expect(selected.classes()).not.toContain('text-txt3')

    const selectedName = agentNameClass(wrapper, 'bob')
    expect(selectedName).toContain('text-txt')
    expect(selectedName).not.toContain('text-txt2')
    expect(selectedName).not.toContain('text-txt3')
    expect(selectedName).not.toMatch(/font-(medium|semibold|bold)/)

    const unselected = findAgentRow(wrapper, 'alice')!
    expect(unselected.classes()).not.toContain('bg-accent-dim')
    expect(unselected.classes()).not.toContain('text-txt3')

    const unselectedName = agentNameClass(wrapper, 'alice')
    expect(unselectedName).toContain('text-txt')
    expect(unselectedName).not.toContain('text-txt2')
    expect(unselectedName).not.toContain('text-txt3')
  })

  it('分组名保持 text-txt2，有 parentAgent 时树内不渲染 reportsTo', () => {
    const wrapper = mountSidebar()

    const group = wrapper
      .findAll('[data-org-kind="group"]')
      .find((r) => r.text().includes('开发部门'))!
    const groupName = group.find('.truncate.font-medium.text-txt2')
    expect(groupName.exists()).toBe(true)
    expect(groupName.classes()).toContain('text-txt2')
    expect(groupName.classes()).not.toContain('text-txt')

    const bob = findAgentRow(wrapper, 'bob')!
    expect(bob.text()).not.toMatch(/上级|reportsTo|Reports to/i)
    const reportsTo = bob
      .findAll('span')
      .find((s) => s.classes().includes('text-txt3') && s.text().includes('alice'))
    expect(reportsTo).toBeFalsy()
  })
})

// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { createRouter, createMemoryHistory } from 'vue-router'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import ProjectAgentsPanel from './ProjectAgentsPanel.vue'

async function mountPanel(projectId = 'proj-a') {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/', component: { template: '<div/>' } }],
  })
  await router.push({ path: '/' })
  await router.isReady()
  return mount(ProjectAgentsPanel, {
    props: { projectId },
    global: {
      plugins: [i18n, router],
      stubs: {
        AgentStudioView: {
          props: ['projectId', 'embedded'],
          template:
            '<div data-testid="agents-studio-stub">studio:{{ projectId }}:{{ embedded }}</div>',
        },
      },
    },
  })
}

describe('ProjectAgentsPanel embeds Studio only (g1.1 / g1.2)', () => {
  it('has no outer meta|chat-test subtabs and always embeds AgentStudioView', async () => {
    const wrapper = await mountPanel()
    await flushPromises()
    expect(wrapper.find('[data-testid="project-agents-subtabs"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="project-agents-subtab-meta"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="project-agents-subtab-test"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="project-agents-chat-test"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="agents-chat-tester"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="agents-studio-stub"]').text()).toBe('studio:proj-a:true')
    wrapper.unmount()
  })
})

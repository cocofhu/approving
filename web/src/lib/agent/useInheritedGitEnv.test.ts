// @vitest-environment happy-dom
import { defineComponent, nextTick, ref } from 'vue'
import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useInheritedGitEnv } from './useInheritedGitEnv'

const getProjectSharedAgentConfig = vi.fn()

vi.mock('@/lib/api/api', () => ({
  api: {
    getProjectSharedAgentConfig: (...args: unknown[]) => getProjectSharedAgentConfig(...args),
  },
}))

function mountHook(projectId: string | undefined) {
  const pid = ref(projectId)
  const Comp = defineComponent({
    setup() {
      const { inheritedEnv } = useInheritedGitEnv(pid)
      return { inheritedEnv, pid }
    },
    template: '<div />',
  })
  return { wrapper: mount(Comp), pid }
}

describe('useInheritedGitEnv', () => {
  beforeEach(() => {
    getProjectSharedAgentConfig.mockReset()
  })

  it('无 projectId 时不请求，inheritedEnv 为空', async () => {
    const { wrapper } = mountHook('')
    await nextTick()
    expect(getProjectSharedAgentConfig).not.toHaveBeenCalled()
    expect(wrapper.vm.inheritedEnv).toEqual([])
    wrapper.unmount()
  })

  it('已绑定项目时读取共享 env（g2.1）', async () => {
    getProjectSharedAgentConfig.mockResolvedValue({
      projectId: 'p1',
      env: { GITLAB_TOKEN: '${vars.gitlab_pat}', GIT_REPOS: '${vars.repos}' },
      files: [],
      mcp: [],
      layout: {},
    })
    const { wrapper } = mountHook('p1')
    await vi.waitFor(() => {
      expect(wrapper.vm.inheritedEnv).toEqual([
        { k: 'GITLAB_TOKEN', v: '${vars.gitlab_pat}' },
        { k: 'GIT_REPOS', v: '${vars.repos}' },
      ])
    })
    expect(getProjectSharedAgentConfig).toHaveBeenCalledWith('p1')
    wrapper.unmount()
  })

  it('读取失败则只看本地，不抛错（g2.1）', async () => {
    getProjectSharedAgentConfig.mockRejectedValue(new Error('network'))
    const { wrapper } = mountHook('p-fail')
    await vi.waitFor(() => {
      expect(getProjectSharedAgentConfig).toHaveBeenCalled()
    })
    expect(wrapper.vm.inheritedEnv).toEqual([])
    wrapper.unmount()
  })
})

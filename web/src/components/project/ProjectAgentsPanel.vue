<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import Icon from '@/components/ui/Icon.vue'
import AgentChatTester from '@/components/agent/AgentChatTester.vue'
import AgentStudioView from '@/views/AgentStudioView.vue'
import { api, type CreateAgentTestPayload, type SandboxView } from '@/lib/api/api'

const props = defineProps<{ projectId: string }>()

const { t } = useI18n()
const route = useRoute()

type AgentsSubTab = 'meta' | 'test'

const subTab = ref<AgentsSubTab>('meta')
const agents = ref<{ name: string; projectId?: string }[]>([])

const projectAgents = computed(() =>
  agents.value.filter((a) => a.projectId === props.projectId),
)

/** Current Agent from Studio URL sync, else first project-bound Agent. */
const testProfile = computed(() => {
  const q = typeof route.query.agent === 'string' ? route.query.agent.trim() : ''
  if (q && projectAgents.value.some((a) => a.name === q)) return q
  if (q && agents.value.length === 0) return q
  return projectAgents.value[0]?.name || ''
})

const subTabs = computed(() => [
  { k: 'meta' as const, l: t('pages.projectDetail.agents.metaTab') },
  { k: 'test' as const, l: t('pages.projectDetail.agents.testTab') },
])

async function loadAgents() {
  try {
    const list = await api.listAgents()
    agents.value = list.map((a) => ({ name: a.name, projectId: a.projectId }))
  } catch {
    agents.value = []
  }
}

async function createProjectContextTest(
  profile: string,
  payload: CreateAgentTestPayload,
): Promise<SandboxView> {
  return api.createProjectSharedAgentTest(props.projectId, {
    agentName: profile,
    ...(payload.repos ? { repos: payload.repos } : {}),
    ...(payload.repoUrl ? { repoUrl: payload.repoUrl } : {}),
  })
}

watch(
  () => props.projectId,
  () => {
    subTab.value = 'meta'
    void loadAgents()
  },
)

watch(subTab, (next) => {
  if (next === 'test') void loadAgents()
})

onMounted(() => {
  void loadAgents()
})
</script>

<template>
  <div
    class="flex min-h-0 flex-1 flex-col"
    data-testid="project-agents-panel"
  >
    <div
      class="scroll-area flex shrink-0 gap-1 overflow-x-auto border-b border-line px-2"
      data-testid="project-agents-subtabs"
    >
      <button
        v-for="tabItem in subTabs"
        :key="tabItem.k"
        type="button"
        class="shrink-0 whitespace-nowrap px-2.5 py-1.5 text-[12px] transition"
        :class="
          subTab === tabItem.k
            ? 'border-b-2 border-accent text-txt font-semibold'
            : 'text-txt3 hover:text-txt2'
        "
        :data-testid="`project-agents-subtab-${tabItem.k}`"
        @click="subTab = tabItem.k"
      >
        {{ tabItem.l }}
      </button>
    </div>

    <!-- Keep Studio mounted so agent selection (route.query.agent) stays synced. -->
    <div
      v-show="subTab === 'meta'"
      class="flex min-h-0 flex-1 flex-col"
      data-testid="project-agents-meta"
    >
      <AgentStudioView :project-id="projectId" embedded />
    </div>

    <div
      v-if="subTab === 'test'"
      class="rounded-lg flex min-h-0 flex-1 flex-col overflow-hidden border border-b-0 border-line bg-surface shadow-[var(--shadow-card)]"
      data-testid="project-agents-chat-test"
    >
      <div
        v-if="!testProfile"
        class="flex flex-1 flex-col items-center justify-center gap-2 px-4 text-center text-[13px] text-txt3"
        data-testid="project-agents-chat-test-empty"
      >
        <Icon name="robot" :size="20" />
        <p>{{ t('pages.projectDetail.agents.noAgentForTest') }}</p>
      </div>
      <AgentChatTester
        v-else
        :key="testProfile"
        :profile="testProfile"
        :home-project-id="projectId"
        :create-test="createProjectContextTest"
      />
    </div>
  </div>
</template>

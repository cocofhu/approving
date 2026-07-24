<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '../ui/Icon.vue'

// phase:
//  - 'pending'  节点尚未进入澄清(队列中),静态等待提示
//  - 'starting' 已进入,正在创建沙箱 / 建立 ACP / 生成第一轮问题,展示分阶段动效
const props = defineProps<{ phase: 'pending' | 'starting' }>()

const { t } = useI18n()

// Cycle through the real boot stages so the panel feels alive and hints at what
// is happening, mirroring the Agent 测试沙箱 starting loader.
const STARTING_STEPS = computed(() => [
  { title: t('pages.clarifyBootLoader.stepSandbox.title'), hint: t('pages.clarifyBootLoader.stepSandbox.hint') },
  { title: t('pages.clarifyBootLoader.stepRuntime.title'), hint: t('pages.clarifyBootLoader.stepRuntime.hint') },
  { title: t('pages.clarifyBootLoader.stepAcp.title'), hint: t('pages.clarifyBootLoader.stepAcp.hint') },
  { title: t('pages.clarifyBootLoader.stepQuestions.title'), hint: t('pages.clarifyBootLoader.stepQuestions.hint') },
])
const step = ref(0)
let timer: number | undefined

function startCycling() {
  step.value = 0
  if (timer) clearInterval(timer)
  timer = window.setInterval(() => {
    // Advance but hold on the last step so it doesn't loop back to the start.
    if (step.value < STARTING_STEPS.value.length - 1) step.value++
  }, 2600)
}
function stopCycling() {
  if (timer) clearInterval(timer)
  timer = undefined
}

watch(
  () => props.phase,
  (p) => (p === 'starting' ? startCycling() : stopCycling()),
)
onMounted(() => {
  if (props.phase === 'starting') startCycling()
})
onBeforeUnmount(stopCycling)
</script>

<template>
  <div class="flex h-full flex-col items-center justify-center gap-3 px-6 text-center">
    <template v-if="phase === 'pending'">
      <Icon name="dot" :size="22" class="animate-pulseglow text-n-clarify" />
      <p class="text-[13px] text-txt2">{{ t('pages.clarifyBootLoader.pending') }}</p>
      <p class="max-w-md text-[11px] text-txt3/70">{{ t('pages.clarifyBootLoader.pendingHint') }}</p>
    </template>
    <template v-else>
      <Icon name="spinner" :size="30" class="animate-spin text-n-clarify" />
      <Transition name="startfade" mode="out-in">
        <p :key="step" class="text-[13px] text-txt2">{{ STARTING_STEPS[step].title }}</p>
      </Transition>
      <Transition name="startfade" mode="out-in">
        <p :key="'h' + step" class="max-w-md text-[12px] text-txt3">{{ STARTING_STEPS[step].hint }}</p>
      </Transition>
      <div class="mt-1 flex items-center gap-1.5">
        <span
          v-for="(_, i) in STARTING_STEPS"
          :key="i"
          class="h-1.5 rounded-full transition-all duration-300"
          :class="i === step ? 'w-5 bg-n-clarify' : i < step ? 'w-1.5 bg-n-clarify/50' : 'w-1.5 bg-line'"
        />
      </div>
    </template>
  </div>
</template>

<style scoped>
.startfade-enter-active,
.startfade-leave-active {
  transition: opacity 0.25s ease, transform 0.25s ease;
}
.startfade-enter-from {
  opacity: 0;
  transform: translateY(4px);
}
.startfade-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}
</style>

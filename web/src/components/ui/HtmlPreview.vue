<script setup lang="ts">
import { ref, watch, onMounted, onBeforeUnmount, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { injectDemoScrollbarStyles } from '@/lib/demoScrollbar'
import {
  SANDBOX_ATTR,
  INLINE_FALLBACK_HEIGHT,
  RESIZE_HEIGHT_EPSILON,
  RESIZE_TIMEOUT_MS,
  HTML_PREVIEW_DEFAULT_TOOLBAR_PX,
  contentFitPreviewCapPx,
  createInstanceId,
  injectPreviewScripts,
  isValidResizeMessage,
  parseResizeMessage,
  isValidInspectPickMessage,
  parseInspectPickMessage,
  buildInspectCommand,
} from '@/lib/htmlPreviewSandbox'
import Icon from './Icon.vue'
import AppModal from './AppModal.vue'

const props = withDefaults(
  defineProps<{
    html: string
    enlargeable?: boolean
    mode?: 'default' | 'inline' | 'demo'
    modalTitle?: string
    /**
     * When true with mode=default, iframe height follows document scrollHeight
     * (toolbar kept). Default false preserves h-full / flex-1 fill behavior.
     */
    fitContent?: boolean
    /**
     * Optional vh cap for measured content height (fillPreview content-fit).
     * When measured height exceeds the cap (minus chrome), iframe height is
     * clamped and scrolling=auto so content stays readable inside the shell.
     */
    maxContentHeightVh?: number
    /**
     * Extra chrome above this preview inside the content-fit shell
     * (e.g. GateApproval reviewing-upstream strip). Subtracted from the vh cap
     * so toolbar + strip + iframe stay within the shell without nested scroll.
     */
    contentHeightOffsetPx?: number
    /**
     * When true, inject inspect script and show a pick-element control.
     * Opaque-origin iframe posts selector + element screenshot to parent.
     */
    inspectable?: boolean
    /**
     * Fill the parent box instead of sizing from document scrollHeight.
     * Used by Run-detail mobile visual gates: shell height comes from flex
     * remaining space; iframe scrolls internally (short docs leave blank space).
     */
    fillParent?: boolean
  }>(),
  {
    enlargeable: true,
    mode: 'default',
    fitContent: false,
    contentHeightOffsetPx: 0,
    inspectable: false,
    fillParent: false,
  },
)

const emit = defineEmits<{
  (
    e: 'pick',
    payload: { selector: string; tagName: string; imageDataUrl: string },
  ): void
}>()

const { t } = useI18n()
const device = ref<'desktop' | 'mobile'>('desktop')
const big = ref(false)
const modalHtml = ref('')
const iframeRef = ref<HTMLIFrameElement | null>(null)
const toolbarRef = ref<HTMLElement | null>(null)
const contentHeight = ref(INLINE_FALLBACK_HEIGHT)
const contentDegraded = ref(false)
/** True when measured height was clamped to maxContentHeightVh. */
const contentCapped = ref(false)
/** Steady-state: suppress micro-resize jitter and height transition. */
const heightStable = ref(false)
/** First inline/content-fit convergence from fallback height. */
const isFirstHeightConvergence = ref(true)
const inspecting = ref(false)
const instanceId = createInstanceId()
let resizeTimeout: ReturnType<typeof setTimeout> | undefined
let pendingResizeHeight: number | null = null
let resizeRafId: number | null = null

function computeHtmlHash(html: string): string {
  let hash = 0
  for (let i = 0; i < html.length; i++) {
    hash = (Math.imul(31, hash) + html.charCodeAt(i)) | 0
  }
  return `${html.length}-${hash}`
}

const lastHtmlHash = ref(computeHtmlHash(props.html || ''))

/** Fixed sandbox policy — see htmlPreviewSandbox.ts (no allow-same-origin). */
const sandboxAttr = SANDBOX_ATTR

const needsContentHeight = computed(
  () =>
    !props.fillParent &&
    (props.mode === 'inline' || (props.mode === 'default' && props.fitContent)),
)

const needsMessageListener = computed(
  () => needsContentHeight.value || props.inspectable,
)

const iframeScrolling = computed(() => {
  if (props.fillParent) return 'auto'
  if (!needsContentHeight.value) return undefined
  return contentDegraded.value || contentCapped.value ? 'auto' : 'no'
})

/** Effective iframe pixel cap after deducting toolbar / outer chrome. */
function resolveContentHeightCapPx(): number | null {
  const vh = props.maxContentHeightVh
  if (vh == null || vh <= 0) return null
  let cap = contentFitPreviewCapPx(window.innerHeight, vh)
  if (props.mode === 'default') {
    const toolbarH = toolbarRef.value?.offsetHeight || HTML_PREVIEW_DEFAULT_TOOLBAR_PX
    cap -= toolbarH
  }
  const offset = props.contentHeightOffsetPx ?? 0
  if (offset > 0) cap -= offset
  return Math.max(INLINE_FALLBACK_HEIGHT, cap)
}

function applyMeasuredHeight(measured: number) {
  const current = contentHeight.value
  const delta = Math.abs(measured - current)

  if (heightStable.value && delta < RESIZE_HEIGHT_EPSILON) {
    return
  }

  if (
    isFirstHeightConvergence.value &&
    current === INLINE_FALLBACK_HEIGHT &&
    measured < current
  ) {
    return
  }

  const cap = resolveContentHeightCapPx()
  let next = measured
  if (cap != null && measured > cap) {
    next = cap
    contentCapped.value = true
    contentDegraded.value = false
  } else {
    contentCapped.value = false
    contentDegraded.value = false
  }

  if (heightStable.value && Math.abs(next - current) < RESIZE_HEIGHT_EPSILON) {
    return
  }

  contentHeight.value = next

  if (
    isFirstHeightConvergence.value &&
    next > INLINE_FALLBACK_HEIGHT + RESIZE_HEIGHT_EPSILON
  ) {
    window.setTimeout(() => {
      heightStable.value = true
      isFirstHeightConvergence.value = false
    }, 180)
  }
}

const deviceHint = computed(() =>
  device.value === 'mobile' ? t('common.htmlPreview.mobileWidth') : t('common.htmlPreview.adaptiveWidth'),
)

const demoModalTitle = computed(() => props.modalTitle || t('common.htmlPreview.title'))

const demoSrcdoc = computed(() => injectDemoScrollbarStyles(props.html))

const previewSrcdoc = computed(() =>
  injectPreviewScripts(props.html, instanceId, {
    resize: needsContentHeight.value,
    inspect: props.inspectable,
  }),
)

/** Mount key — remount iframe only on structural changes, not html content updates. */
const iframeMountKey = computed(
  () =>
    `${props.mode}-${props.fitContent}-${props.fillParent}-${props.inspectable}-${props.maxContentHeightVh ?? ''}-${instanceId}`,
)

function openEnlarge() {
  modalHtml.value = injectDemoScrollbarStyles(props.html)
  big.value = true
}

function closeEnlarge() {
  big.value = false
  modalHtml.value = ''
}

function postInspectCommand(enabled: boolean) {
  const win = iframeRef.value?.contentWindow
  if (!win || !props.inspectable) return
  win.postMessage(buildInspectCommand(instanceId, enabled), '*')
}

function setInspecting(on: boolean) {
  if (!props.inspectable) return
  inspecting.value = on
  postInspectCommand(on)
}

function toggleInspect() {
  setInspecting(!inspecting.value)
}

defineExpose({ openEnlarge, setInspecting })

function clearResizeTimeout() {
  if (resizeTimeout !== undefined) {
    clearTimeout(resizeTimeout)
    resizeTimeout = undefined
  }
}

function applyContentFallback() {
  contentDegraded.value = true
  contentCapped.value = false
  contentHeight.value = INLINE_FALLBACK_HEIGHT
}

function startResizeTimeout() {
  clearResizeTimeout()
  resizeTimeout = setTimeout(applyContentFallback, RESIZE_TIMEOUT_MS)
}

function scheduleApplyResize(height: number) {
  pendingResizeHeight = height
  if (resizeRafId != null) return
  resizeRafId = requestAnimationFrame(() => {
    resizeRafId = null
    const h = pendingResizeHeight
    pendingResizeHeight = null
    if (h != null) applyMeasuredHeight(h)
  })
}

function handlePreviewMessage(event: MessageEvent) {
  if (!iframeRef.value) return
  if (event.source !== iframeRef.value.contentWindow) return
  if (event.data?.id !== instanceId) return

  if (needsContentHeight.value && isValidResizeMessage(event.data)) {
    const parsed = parseResizeMessage(event.data)
    if (!parsed) return
    clearResizeTimeout()
    scheduleApplyResize(parsed.height)
    return
  }

  if (props.inspectable && isValidInspectPickMessage(event.data)) {
    const parsed = parseInspectPickMessage(event.data)
    if (!parsed) return
    inspecting.value = false
    postInspectCommand(false)
    emit('pick', {
      selector: parsed.selector,
      tagName: parsed.tagName,
      imageDataUrl: parsed.imageDataUrl,
    })
  }
}

function onPreviewLoad() {
  if (needsContentHeight.value) startResizeTimeout()
  if (props.inspectable && inspecting.value) postInspectCommand(true)
}

function resetContentHeightState() {
  contentHeight.value = INLINE_FALLBACK_HEIGHT
  contentDegraded.value = false
  contentCapped.value = false
  heightStable.value = false
  isFirstHeightConvergence.value = true
  clearResizeTimeout()
  if (resizeRafId != null) {
    cancelAnimationFrame(resizeRafId)
    resizeRafId = null
  }
  pendingResizeHeight = null
}

onMounted(() => {
  if (needsMessageListener.value) {
    window.addEventListener('message', handlePreviewMessage)
  }
})

onBeforeUnmount(() => {
  window.removeEventListener('message', handlePreviewMessage)
  clearResizeTimeout()
  if (resizeRafId != null) cancelAnimationFrame(resizeRafId)
})

watch(
  () =>
    [
      props.html,
      props.mode,
      props.fitContent,
      props.fillParent,
      props.maxContentHeightVh,
      props.contentHeightOffsetPx,
      props.inspectable,
    ] as const,
  (next, prev) => {
    const newHash = computeHtmlHash(next[0] || '')
    const htmlUnchanged = newHash === lastHtmlHash.value
    const structuralChanged =
      !prev ||
      next[1] !== prev[1] ||
      next[2] !== prev[2] ||
      next[3] !== prev[3] ||
      next[4] !== prev[4] ||
      next[5] !== prev[5] ||
      next[6] !== prev[6]

    if (structuralChanged && needsContentHeight.value) {
      resetContentHeightState()
      lastHtmlHash.value = newHash
    } else if (!htmlUnchanged) {
      lastHtmlHash.value = newHash
      // Silent srcdoc hot-update: keep iframe mounted, let reactive srcdoc apply.
    }

    if (!props.inspectable) inspecting.value = false
  },
)

watch(device, () => {
  if (props.mode === 'default' && props.fitContent) resetContentHeightState()
})

watch(needsMessageListener, (enabled, wasEnabled) => {
  if (enabled && !wasEnabled) {
    window.addEventListener('message', handlePreviewMessage)
    if (needsContentHeight.value) resetContentHeightState()
  } else if (!enabled && wasEnabled) {
    window.removeEventListener('message', handlePreviewMessage)
    clearResizeTimeout()
  }
})
</script>

<template>
  <!-- Inline/mobile: content-height shell, or fillParent (h-full + iframe scroll). -->
  <div
    v-if="mode === 'inline'"
    class="w-full overflow-x-hidden"
    :class="fillParent ? 'flex h-full min-h-0 flex-col' : ''"
    :data-fill-parent="fillParent ? '1' : '0'"
  >
    <div
      v-if="inspectable"
      class="flex shrink-0 items-center gap-2 border-b border-line px-3 py-2"
      data-testid="html-preview-inspect-bar"
    >
      <button
        type="button"
        class="flex items-center gap-1 rounded-md border px-2 py-1 text-[11px] transition-colors"
        :class="
          inspecting
            ? 'border-accent bg-accent-dim/50 text-accent'
            : 'border-line text-txt2 hover:text-txt'
        "
        data-testid="html-preview-inspect-toggle"
        @click="toggleInspect"
      >
        <Icon name="crosshair" :size="13" />
        {{
          inspecting
            ? t('pages.appPreview.novnc.cancelInspect')
            : t('pages.appPreview.novnc.inspect')
        }}
      </button>
    </div>
    <div :class="fillParent ? 'min-h-0 flex-1' : ''">
      <iframe
        :key="iframeMountKey"
        ref="iframeRef"
        :srcdoc="previewSrcdoc"
        :sandbox="sandboxAttr"
        :scrolling="iframeScrolling"
        class="w-full border-0 bg-white"
        :class="[
          fillParent ? 'h-full' : '',
          { 'html-preview-height-transition': needsContentHeight && !heightStable },
        ]"
        :style="fillParent ? undefined : { height: contentHeight + 'px' }"
        :title="t('common.htmlPreview.title')"
        @load="onPreviewLoad"
      />
    </div>
  </div>

  <!-- Demo mode: compact card preview + simplified enlarge modal (no device toggle) -->
  <template v-else-if="mode === 'demo'">
    <iframe
      :key="iframeMountKey"
      :srcdoc="demoSrcdoc"
      :sandbox="sandboxAttr"
      class="block h-[140px] w-full border-0 bg-white"
      :title="demoModalTitle"
    />
    <AppModal
      v-if="enlargeable"
      :open="big"
      :title="demoModalTitle"
      :width="900"
      @close="closeEnlarge"
    >
      <iframe
        v-if="modalHtml"
        :key="iframeMountKey + '-modal'"
        :srcdoc="modalHtml"
        :sandbox="sandboxAttr"
        class="block h-[70vh] w-full border-0 bg-white"
        :title="demoModalTitle"
      />
    </AppModal>
  </template>

  <!-- Default desktop preview with device toggle -->
  <div
    v-else
    class="flex flex-col"
    :class="fitContent ? '' : 'h-full min-h-0'"
  >
    <div
      ref="toolbarRef"
      class="flex shrink-0 items-center gap-2 border-b border-line px-3 py-2"
      data-testid="html-preview-toolbar"
    >
      <div class="flex overflow-hidden rounded-md border border-line text-[11px]">
        <button
          class="flex items-center gap-1 px-2 py-1 transition-colors"
          :class="device === 'desktop' ? 'bg-accent-dim/50 text-accent' : 'text-txt3 hover:text-txt'"
          @click="device = 'desktop'"
        >
          <Icon name="monitor" :size="13" />{{ t('common.htmlPreview.desktop') }}
        </button>
        <button
          class="flex items-center gap-1 border-l border-line px-2 py-1 transition-colors"
          :class="device === 'mobile' ? 'bg-accent-dim/50 text-accent' : 'text-txt3 hover:text-txt'"
          @click="device = 'mobile'"
        >
          <Icon name="mobile" :size="13" />{{ t('common.htmlPreview.mobile') }}
        </button>
      </div>
      <span class="text-[10px] text-txt3">{{ deviceHint }}</span>
      <button
        v-if="inspectable"
        type="button"
        class="flex items-center gap-1 rounded-md border px-2 py-1 text-[11px] transition-colors"
        :class="
          inspecting
            ? 'border-accent bg-accent-dim/50 text-accent'
            : 'border-line text-txt2 hover:text-txt'
        "
        data-testid="html-preview-inspect-toggle"
        @click="toggleInspect"
      >
        <Icon name="crosshair" :size="13" />
        {{
          inspecting
            ? t('pages.appPreview.novnc.cancelInspect')
            : t('pages.appPreview.novnc.inspect')
        }}
      </button>
      <button
        v-if="enlargeable"
        class="ml-auto flex items-center gap-1 rounded-md border border-line px-2 py-1 text-[11px] text-txt2 transition-colors hover:text-txt"
        :title="t('common.htmlPreview.enlargeTitle')"
        @click="big = true"
      >
        <Icon name="expand" :size="13" />{{ t('common.htmlPreview.window') }}
      </button>
    </div>
    <div
      class="bg-elevated"
      :class="[
        fitContent ? 'shrink-0' : 'min-h-0 flex-1 overflow-hidden',
        device === 'mobile' ? 'flex justify-center p-4' : '',
      ]"
    >
      <iframe
        v-if="device === 'desktop'"
        :key="iframeMountKey + '-desktop'"
        ref="iframeRef"
        :srcdoc="inspectable || fitContent ? previewSrcdoc : html"
        :sandbox="sandboxAttr"
        :scrolling="fitContent ? iframeScrolling : undefined"
        class="w-full border-0 bg-white"
        :class="fitContent ? '' : 'h-full'"
        :style="fitContent ? { height: contentHeight + 'px' } : undefined"
        :title="t('common.htmlPreview.title')"
        @load="onPreviewLoad"
      />
      <div
        v-else
        class="w-[390px] shrink-0 overflow-hidden border border-line bg-white shadow-lg"
        :class="fitContent ? '' : 'h-full'"
        :style="fitContent ? { height: contentHeight + 'px' } : undefined"
      >
        <iframe
          :key="iframeMountKey + '-mobile'"
          ref="iframeRef"
          :srcdoc="inspectable || fitContent ? previewSrcdoc : html"
          :sandbox="sandboxAttr"
          :scrolling="fitContent ? iframeScrolling : undefined"
          class="h-full w-full border-0 bg-white"
          :title="t('common.htmlPreview.mobilePreview')"
          @load="onPreviewLoad"
        />
      </div>
    </div>
  </div>

  <!-- Enlarge uses fixed viewport height; nested preview keeps default fill (no fitContent). -->
  <AppModal v-if="enlargeable && mode === 'default'" :open="big" :title="t('common.htmlPreview.title')" :width="1120" @close="big = false">
    <div class="h-[80vh]">
      <HtmlPreview :html="html" :enlargeable="false" :inspectable="inspectable" @pick="emit('pick', $event)" />
    </div>
  </AppModal>
</template>

<style scoped>
.html-preview-height-transition {
  transition: height 180ms ease-out;
}
</style>

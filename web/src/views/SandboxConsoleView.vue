<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, nextTick, watch, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'
import NovncPreviewPanel from '@/components/run/NovncPreviewPanel.vue'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import { api, type SandboxView } from '@/lib/api'
import { copyToClipboard } from '@/lib/copyToClipboard'
import { useToast } from '@/lib/useToast'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const toast = useToast()
const id = Number(route.params.id)

const CONSOLE_TABS = ['terminal', 'ide', 'acp', 'novnc', 'log'] as const
type ConsoleTab = (typeof CONSOLE_TABS)[number]

function initialConsoleTab(): ConsoleTab {
  const q = route.query.tab
  if (typeof q !== 'string') return 'terminal'
  // Legacy ACP native bridge deep link → new unified ACP tab.
  if (q === 'acp-native') return 'acp'
  // Legacy embedded AgentChatTester ACP deep link → terminal (do not restore chat).
  if (q === 'acp') return 'terminal'
  if ((CONSOLE_TABS as readonly string[]).includes(q)) {
    return q as ConsoleTab
  }
  return 'terminal'
}

const sandbox = ref<SandboxView | null>(null)
const tab = ref<ConsoleTab>(initialConsoleTab())
// Lazily mount heavy panes on first open, then keep alive (v-show) so
// sessions survive tab switches — same pattern as the terminal pane.
const novncMounted = ref(false)
const ideMounted = ref(false)
const acpMounted = ref(false)

const tabItems = computed(() => [
  { k: 'terminal', l: t('pages.sandboxConsole.tabs.terminal'), i: 'terminal' },
  { k: 'ide', l: t('pages.sandboxConsole.tabs.ide'), i: 'doc' },
  { k: 'acp', l: t('pages.sandboxConsole.tabs.acp'), i: 'robot' },
  { k: 'novnc', l: t('pages.sandboxConsole.tabs.novnc'), i: 'globe' },
  { k: 'log', l: t('pages.sandboxConsole.tabs.log'), i: 'doc' },
])

const sandboxTitle = computed(() => sandbox.value?.name || t('pages.sandboxConsole.fallbackName', { id }))
const showPassword = ref(false)
const passwordCopied = ref(false)

async function copyPassword() {
  const pw = sandbox.value?.password
  if (!pw) return
  const ok = await copyToClipboard(pw)
  if (!ok) {
    toast.error(t('common.toast.copyFailed'))
    return
  }
  passwordCopied.value = true
  window.setTimeout(() => { passwordCopied.value = false }, 1500)
}

// Raw container logs (docker logs): live while running, else archived snapshot.
const log = ref<{ content: string; live: boolean; found: boolean } | null>(null)
const logLoading = ref(false)
async function fetchLog() {
  logLoading.value = true
  try {
    log.value = await api.sandboxLog(id)
  } catch {
    log.value = null
  } finally {
    logLoading.value = false
  }
}
const termHost = ref<HTMLElement | null>(null)
const termStatus = ref<'connecting' | 'open' | 'closed'>('connecting')

const termStatusLabel = computed(() => {
  if (termStatus.value === 'open') return t('pages.sandboxConsole.terminalStatus.open')
  if (termStatus.value === 'closed') return t('pages.sandboxConsole.terminalStatus.closed')
  return t('pages.sandboxConsole.terminalStatus.connecting')
})

let term: Terminal | null = null
let fit: FitAddon | null = null
let ws: WebSocket | null = null
let ro: ResizeObserver | null = null
// Last dimensions sent to the PTY. Used to suppress redundant resize frames:
// every SIGWINCH makes the shell redraw its prompt line, so spurious resizes
// (e.g. the ResizeObserver firing when the terminal is hidden/shown by a tab
// switch) would litter the buffer with duplicated prompt fragments.
let lastCols = 0
let lastRows = 0

async function loadMeta() {
  try {
    // Prefer getSandbox so password is available even if list is filtered/cached.
    sandbox.value = await api.getSandbox(id)
  } catch {
    try {
      const list = await api.listSandboxes()
      sandbox.value = list.find((s) => s.id === id) || null
    } catch {
      sandbox.value = null
    }
  }
}

function initTerminal() {
  if (term || !termHost.value) return
  term = new Terminal({
    fontSize: 12.5,
    fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace',
    cursorBlink: true,
    theme: { background: '#0b0e14', foreground: '#cdd6f4' },
  })
  fit = new FitAddon()
  term.loadAddon(fit)
  term.open(termHost.value)
  doFit()

  ws = new WebSocket(api.sandboxTerminalWsUrl(id))
  ws.binaryType = 'arraybuffer'
  ws.onopen = () => {
    termStatus.value = 'open'
    sendResize()
  }
  ws.onmessage = (ev) => {
    if (typeof ev.data === 'string') {
      try {
        const m = JSON.parse(ev.data)
        if (m.type === 'error') term?.write(`\r\n\x1b[31m${m.data}\x1b[0m\r\n`)
        return
      } catch {
        term?.write(ev.data)
        return
      }
    }
    term?.write(new Uint8Array(ev.data as ArrayBuffer))
  }
  ws.onclose = () => {
    termStatus.value = 'closed'
    term?.write(`\r\n\x1b[33m${t('pages.sandboxConsole.disconnected')}\x1b[0m\r\n`)
  }

  term.onData((d) => {
    if (ws?.readyState === WebSocket.OPEN) ws.send(JSON.stringify({ type: 'input', data: d }))
  })
}

// termVisible reports whether the terminal pane is actually laid out. When a
// tab switch hides it via v-show (display:none) the host collapses to 0×0;
// fitting/resizing then would compute a bogus geometry and reflow the shell.
function termVisible(): boolean {
  const el = termHost.value
  return !!el && el.clientWidth > 0 && el.clientHeight > 0
}
function doFit() {
  if (!fit || !termVisible()) return
  try {
    fit.fit()
  } catch {
    /* host not laid out yet */
  }
}
function sendResize() {
  if (!term || ws?.readyState !== WebSocket.OPEN || !termVisible()) return
  if (term.cols === lastCols && term.rows === lastRows) return
  lastCols = term.cols
  lastRows = term.rows
  ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }))
}

watch(tab, (t) => {
  if (t === 'terminal') nextTick(() => { initTerminal(); doFit() })
  if (t === 'ide') ideMounted.value = true
  if (t === 'acp') acpMounted.value = true
  if (t === 'novnc') novncMounted.value = true
  if (t === 'log') fetchLog()
})

onMounted(async () => {
  tab.value = initialConsoleTab()
  if (tab.value === 'ide') ideMounted.value = true
  if (tab.value === 'acp') acpMounted.value = true
  if (tab.value === 'novnc') novncMounted.value = true
  await loadMeta()
  nextTick(() => {
    initTerminal()
    ro = new ResizeObserver(() => {
      doFit()
      sendResize()
    })
    if (termHost.value) ro.observe(termHost.value)
  })
})

onBeforeUnmount(() => {
  ro?.disconnect()
  ws?.close()
  term?.dispose()
})
</script>

<template>
  <div class="flex h-full flex-col">
    <div class="flex h-12 shrink-0 items-center gap-3 border-b border-line px-4">
      <button class="flex items-center gap-1 text-[13px] text-txt3 hover:text-txt" @click="router.push('/sandboxes')">
        <Icon name="arrow-left" :size="15" />{{ t('pages.sandboxConsole.back') }}
      </button>
      <span class="text-[13px] font-medium text-txt">{{ sandboxTitle }}</span>
      <span v-if="sandbox" class="chip border-line text-txt3">{{ sandbox.profile }}</span>
      <div
        v-if="sandbox?.password"
        class="ml-2 flex max-w-[min(28rem,50vw)] items-center gap-1.5 rounded border border-line bg-surface px-2 py-0.5 text-[11px] text-txt3"
        :title="t('pages.sandboxConsole.passwordHint')"
      >
        <span class="shrink-0">{{ t('pages.sandboxConsole.password') }}</span>
        <code class="truncate font-mono text-txt2">{{ showPassword ? sandbox.password : '••••••••' }}</code>
        <button type="button" class="shrink-0 text-txt3 hover:text-txt" @click="showPassword = !showPassword">
          {{ showPassword ? t('pages.sandboxConsole.hidePassword') : t('pages.sandboxConsole.showPassword') }}
        </button>
        <button type="button" class="shrink-0 text-accent hover:underline" @click="copyPassword">
          {{ passwordCopied ? t('pages.sandboxConsole.copied') : t('pages.sandboxConsole.copyPassword') }}
        </button>
      </div>

      <div class="ml-4 flex gap-1">
        <button
          v-for="item in tabItems"
          :key="item.k"
          class="flex items-center gap-1 rounded px-3 py-1 text-[12px] transition"
          :class="tab === item.k ? 'bg-accent-dim text-txt' : 'text-txt3 hover:text-txt2'"
          @click="tab = item.k as any"
        ><Icon :name="item.i" :size="13" />{{ item.l }}</button>
      </div>

      <span
        v-if="tab === 'terminal'"
        class="ml-auto chip"
        :class="termStatus === 'open' ? 'border-ok/30 text-ok' : termStatus === 'closed' ? 'border-err/30 text-err' : 'border-line text-txt3'"
      >{{ termStatusLabel }}</span>
    </div>

    <div class="min-h-0 flex-1">
      <!-- terminal kept mounted; toggled with the IDE iframe -->
      <div v-show="tab === 'terminal'" class="h-full bg-[#0b0e14] p-2">
        <div ref="termHost" class="h-full w-full" />
      </div>

      <div v-show="tab === 'ide'" class="h-full">
        <iframe
          v-if="ideMounted && sandbox?.hasCodeServer"
          :src="api.sandboxIdeUrl(id)"
          class="h-full w-full border-0 bg-white"
          title="code-server"
        />
        <div
          v-else-if="ideMounted"
          class="flex h-full items-center justify-center px-6 text-center text-sm text-txt3"
        >
          {{ t('pages.sandboxConsole.ideUnavailable') }}
        </div>
      </div>

      <!-- ACP: in-container acp-bridge web UI (8765); was acp-native -->
      <div v-show="tab === 'acp'" class="h-full">
        <iframe
          v-if="acpMounted && sandbox?.hasAcp"
          :src="api.sandboxBridgeUrl(id)"
          class="h-full w-full border-0 bg-white"
          title="ACP bridge"
        />
        <div
          v-else-if="acpMounted"
          class="flex h-full items-center justify-center px-6 text-center text-sm text-txt3"
        >
          {{ t('pages.sandboxConsole.acpNativeUnavailable') }}
        </div>
      </div>

      <!-- noVNC browser: sandbox-scoped desktop; lazy mount + keep-alive -->
      <div v-show="tab === 'novnc'" class="h-full">
        <NovncPreviewPanel
          v-if="novncMounted"
          :sandbox-id="id"
          fill
        />
      </div>

      <!-- container logs (docker logs): live or archived snapshot -->
      <div v-show="tab === 'log'" class="flex h-full flex-col">
        <div class="flex items-center gap-2 border-b border-line px-3 py-1.5 text-[11px] text-txt3">
          <Icon name="terminal" :size="12" />
          <span>{{ t('pages.sandboxConsole.logTitle') }}</span>
          <span v-if="log?.live" class="inline-flex items-center rounded-full border border-accent/40 bg-accent-dim px-2 py-0.5 text-[10px] text-accent">{{ t('pages.runDetail.sandboxLog.live') }}</span>
          <span v-else-if="log?.found" class="chip">{{ t('pages.runDetail.sandboxLog.archived') }}</span>
          <div class="flex-1" />
          <button class="text-txt3 hover:text-txt" :title="t('common.buttons.refresh')" @click="fetchLog"><Icon name="refresh" :size="12" /></button>
        </div>
        <div class="scroll-area min-h-0 flex-1 overflow-auto bg-[#0b0e14] p-3">
          <pre v-if="log?.content" class="whitespace-pre-wrap break-all font-mono text-[11px] leading-relaxed text-[#cdd6f4]">{{ log.content }}</pre>
          <div v-else class="flex h-full items-center justify-center text-center text-[12px] text-[#cdd6f4]">
            {{ logLoading ? t('common.buttons.loading') : t('pages.sandboxConsole.logEmpty') }}
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

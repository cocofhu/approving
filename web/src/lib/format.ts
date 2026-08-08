import { i18n } from './i18n'
import type { RunOrigin } from './types'

/** Run-state trigger codes + historical display aliases → i18n keys (not Triggers product page). */
const TRIGGER_LABEL_KEYS: Record<string, string> = {
  manual: 'common.runTrigger.manual',
  api: 'common.runTrigger.api',
  pm_mcp: 'common.runTrigger.pmMcp',
  手动触发: 'common.runTrigger.manual',
  'API 触发': 'common.runTrigger.api',
  'PM MCP': 'common.runTrigger.pmMcp',
}

export function formatTrigger(raw: string): string {
  const labelKey = TRIGGER_LABEL_KEYS[raw]
  return labelKey ? i18n.global.t(labelKey) : raw
}

/** Origin channel codes → i18n keys. Unknown channels display their own code. */
const ORIGIN_CHANNEL_KEYS: Record<string, string> = {
  qq: 'common.runOrigin.qq',
}

function originChannelLabel(channel?: string): string {
  const code = (channel || '').trim().toLowerCase()
  if (!code) return i18n.global.t('common.runOrigin.chat')
  const key = ORIGIN_CHANNEL_KEYS[code]
  return key ? i18n.global.t(key) : code.toUpperCase()
}

/**
 * Renders where a run was dispatched from, for the run list. Empty for runs
 * started in the web UI, which is what keeps the chip off rows that have no
 * conversation behind them.
 */
export function formatRunOrigin(origin?: RunOrigin): string {
  if (!origin?.conversationId) return ''
  const channel = originChannelLabel(origin.channel)
  const user = (origin.externalUserId || '').trim()
  if (!user) return i18n.global.t('common.runOrigin.label', { channel })
  return i18n.global.t('common.runOrigin.labelWithUser', { channel, user })
}

/** The full origin, for the chip's tooltip — the chip itself stays short. */
export function formatRunOriginTitle(origin?: RunOrigin): string {
  if (!origin?.conversationId) return ''
  return i18n.global.t('common.runOrigin.title', {
    channel: originChannelLabel(origin.channel),
    conversation: origin.conversationId,
  })
}

export function fmtTime(iso: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

export function fmtDuration(sec: number): string {
  if (sec == null) return '—'
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  const s = Math.floor(sec % 60)
  const p = (n: number) => String(n).padStart(2, '0')
  return h > 0 ? `${p(h)}:${p(m)}:${p(s)}` : `${p(m)}:${p(s)}`
}

/**
 * Compact duration for Run stats KPI main values (Demo-aligned).
 * 0 → "0s"; ≥1h → two decimals + "h" (3703→1.03h); else one decimal + "m" (3458→57.6m).
 */
export function fmtCompactDuration(sec: number): string {
  if (sec == null || !Number.isFinite(sec)) return '—'
  const n = Math.max(0, sec)
  if (n === 0) return '0s'
  if (n >= 3600) return `${(n / 3600).toFixed(2)}h`
  return `${(n / 60).toFixed(1)}m`
}

const SEPARATORS = new Set([' ', '\u3000', '\n', '\t', '/', '-', '_', '|', '·', '。', '，', ',', '.', ';', '；', '、', ':', '：'])
const isHan = (c: string) => c >= '\u4e00' && c <= '\u9fff'
const isTokenChar = (c: string) => /[0-9A-Za-z]/.test(c)

/**
 * Shorten a label without cutting a word — or a character — in half. Slicing by
 * code unit split surrogate pairs into replacement squares and ended titles at
 * 「快模型和 wo」, which is unreadable in a list where the title is the only way
 * to tell two runs apart. Counting by code point and walking back to a word
 * boundary costs a few characters and keeps the label meaningful.
 */
export function truncateText(s: string, maxLen: number): string {
  if (!s) return s
  const chars = Array.from(s)
  if (chars.length <= maxLen) return s
  // A cut between two Han characters is already at a boundary: Chinese does not
  // space its words. Only a Latin token needs walking back.
  if (isTokenChar(chars[maxLen - 1]) && isTokenChar(chars[maxLen])) {
    for (let i = maxLen - 1; i >= 0; i--) {
      if (SEPARATORS.has(chars[i])) {
        const kept = chars.slice(0, i).join('').trimEnd()
        if (kept) return kept + '…'
        break
      }
      if (isHan(chars[i])) return chars.slice(0, i + 1).join('') + '…'
    }
  }
  return chars.slice(0, maxLen).join('').trimEnd() + '…'
}

export function relTime(iso: string): string {
  if (!iso) return ''
  const diff = (Date.now() - new Date(iso).getTime()) / 1000
  if (diff < 60) return i18n.global.t('common.relTime.justNow')
  if (diff < 3600) {
    return i18n.global.t('common.relTime.minutesAgo', { n: Math.floor(diff / 60) })
  }
  if (diff < 86400) {
    return i18n.global.t('common.relTime.hoursAgo', { n: Math.floor(diff / 3600) })
  }
  return i18n.global.t('common.relTime.daysAgo', { n: Math.floor(diff / 86400) })
}

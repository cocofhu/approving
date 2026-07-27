import { computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { isValidRunTag, parseTagQuery, serializeTagQuery } from '@/lib/runTags'

/** Trim + dedupe without validity checks (for detecting dirty URL segments). */
function looseParseTagQuery(raw: string): string[] {
  if (!raw) return []
  const seen = new Set<string>()
  const out: string[] = []
  for (const part of raw.split(',')) {
    const tag = part.trim()
    if (!tag || seen.has(tag)) continue
    seen.add(tag)
    out.push(tag)
  }
  return out
}

/**
 * Selected run tags backed by URL `?tag=` (comma-separated AND filter).
 * Invalid segments are filtered on read/write to match backend NormalizeRunTags.
 */
export function useTagFilter() {
  const route = useRoute()
  const router = useRouter()

  const selectedTags = computed<string[]>({
    get: () => parseTagQuery(typeof route.query.tag === 'string' ? route.query.tag : ''),
    set: (value) => {
      const query = { ...route.query }
      const next = serializeTagQuery(value)
      if (next) query.tag = next
      else delete query.tag
      void router.replace({ query })
    },
  })

  // Drop illegal URL segments so button count matches backend filter semantics.
  watch(
    () => (typeof route.query.tag === 'string' ? route.query.tag : ''),
    (raw) => {
      if (!raw) return
      const loose = looseParseTagQuery(raw)
      const cleaned = parseTagQuery(raw)
      if (loose.length === cleaned.length && loose.every((t, i) => t === cleaned[i])) return
      const query = { ...route.query }
      if (cleaned.length) query.tag = cleaned.join(',')
      else delete query.tag
      void router.replace({ query })
    },
    { immediate: true },
  )

  function addTag(tag: string) {
    const trimmed = tag.trim()
    if (!trimmed || !isValidRunTag(trimmed)) return
    if (selectedTags.value.includes(trimmed)) return
    selectedTags.value = [...selectedTags.value, trimmed]
  }

  function removeTag(tag: string) {
    selectedTags.value = selectedTags.value.filter((item) => item !== tag)
  }

  function toggleTag(tag: string) {
    const trimmed = tag.trim()
    if (!trimmed) return
    if (selectedTags.value.includes(trimmed)) removeTag(trimmed)
    else addTag(trimmed)
  }

  return { selectedTags, addTag, removeTag, toggleTag }
}

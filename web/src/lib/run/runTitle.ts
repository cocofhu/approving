/** Human-readable run title. Repo JSON dumps become repo names; other raw JSON is hidden. */
export function displayRunTitle(raw: string | undefined | null): string {
  const s = String(raw ?? '').trim()
  if (!s) return ''
  const repos = reposTitleFromValue(s)
  if (repos) return repos
  if (s.startsWith('[') || s.startsWith('{')) return ''
  return s
}

function reposTitleFromValue(raw: string): string {
  if (!raw.startsWith('[')) return ''
  try {
    const parsed = JSON.parse(raw) as unknown
    return formatRepoNames(repoNames(parsed))
  } catch {
    return ''
  }
}

function repoNames(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  const names: string[] = []
  for (const item of value) {
    if (!item || typeof item !== 'object') continue
    const rec = item as { name?: unknown; url?: unknown }
    const name = String(rec.name ?? '').trim()
    if (name) {
      names.push(name)
      continue
    }
    const url = String(rec.url ?? '').trim()
    const fromUrl = url.split('/').pop()?.replace(/\.git$/i, '') ?? ''
    if (fromUrl) names.push(fromUrl)
  }
  return names
}

function formatRepoNames(names: string[]): string {
  if (names.length === 0) return ''
  if (names.length === 1) return names[0]
  if (names.length === 2) return `${names[0]} · ${names[1]}`
  return `${names[0]} · ${names[1]} 等 ${names.length} 个仓库`
}

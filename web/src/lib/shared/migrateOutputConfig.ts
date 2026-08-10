/** Migrate legacy config.result to config.results for output nodes. */
export function migrateOutputConfig(config: Record<string, unknown>): {
  config: Record<string, unknown>
  migrated: boolean
} {
  const cfg = { ...config }
  if (Array.isArray(cfg.results)) {
    return { config: cfg, migrated: false }
  }
  const result = (cfg.result ?? '').toString().trim()
  if (result) {
    cfg.results = [result]
    return { config: cfg, migrated: true }
  }
  cfg.results = []
  const migrated = 'result' in config
  return { config: cfg, migrated }
}

/** Apply migration to every output node in a workflow graph. */
export function migrateOutputNodes(nodes: { type: string; config?: Record<string, unknown> }[]): boolean {
  let any = false
  for (const n of nodes) {
    if (n.type !== 'output' || !n.config) continue
    const { config, migrated } = migrateOutputConfig(n.config)
    n.config = config
    if (migrated) any = true
  }
  return any
}

/** Migrate then strip legacy result — use before capturing editor baseline. */
export function migrateAndCleanOutputNodes(
  nodes: { type: string; config?: Record<string, unknown> }[],
): boolean {
  const migrated = migrateOutputNodes(nodes)
  for (const n of nodes) {
    if (n.type === 'output' && n.config) {
      n.config = cleanOutputConfigForSave(n.config)
    }
  }
  return migrated
}

/** Strip legacy config.result before persisting. */
export function cleanOutputConfigForSave(config: Record<string, unknown>): Record<string, unknown> {
  const cfg = { ...config }
  delete cfg.result
  if (!Array.isArray(cfg.results)) cfg.results = []
  return cfg
}

import { sidebarNavGroups } from './sidebarNav'
import { settingsNavItems } from './settingsNav'

/** Unique sidebar + settings paths for e2e dummy routers (plan g2.4 / g3.1). */
export function shellNavPaths(): string[] {
  return [
    ...new Set([
      ...sidebarNavGroups.flatMap((g) => g.items.map((i) => i.to)),
      ...settingsNavItems.map((i) => i.to),
    ]),
  ]
}

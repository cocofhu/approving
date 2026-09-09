export type SidebarNavItem = { to: string; icon: string; labelKey: string }
export type SidebarNavGroup = { titleKey?: string; items: SidebarNavItem[] }

/** Workspace primary nav: Home / Gates / Runs / Settings only (plan g1.1). */
export const sidebarNavGroups: SidebarNavGroup[] = [
  {
    items: [
      { to: '/dashboard', icon: 'dashboard', labelKey: 'nav.dashboard' },
      { to: '/gates', icon: 'gate', labelKey: 'nav.gates' },
      { to: '/runs', icon: 'runs', labelKey: 'nav.runs' },
      { to: '/settings', icon: 'settings', labelKey: 'nav.settings' },
    ],
  },
]

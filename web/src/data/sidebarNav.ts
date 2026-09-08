export type SidebarNavItem = { to: string; icon: string; labelKey: string }
export type SidebarNavGroup = { titleKey?: string; items: SidebarNavItem[] }

/** Workspace primary nav: Home / Gates / Notifications / Settings only (plan g1.1). */
export const sidebarNavGroups: SidebarNavGroup[] = [
  {
    items: [
      { to: '/dashboard', icon: 'dashboard', labelKey: 'nav.dashboard' },
      { to: '/gates', icon: 'gate', labelKey: 'nav.gates' },
      { to: '/notifications', icon: 'bell', labelKey: 'nav.notifications' },
      { to: '/settings', icon: 'settings', labelKey: 'nav.settings' },
    ],
  },
]

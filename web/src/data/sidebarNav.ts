export type SidebarNavItem = { to: string; icon: string; labelKey: string }
export type SidebarNavGroup = { titleKey?: string; items: SidebarNavItem[] }

export const sidebarNavGroups: SidebarNavGroup[] = [
  {
    items: [
      { to: '/dashboard', icon: 'dashboard', labelKey: 'nav.dashboard' },
      { to: '/stats', icon: 'chart', labelKey: 'nav.stats' },
      { to: '/projects', icon: 'folder', labelKey: 'nav.projects' },
      { to: '/runs', icon: 'runs', labelKey: 'nav.runs' },
      { to: '/gates', icon: 'gate', labelKey: 'nav.gates' },
      { to: '/artifacts', icon: 'artifact', labelKey: 'nav.artifacts' },
      { to: '/notifications', icon: 'bell', labelKey: 'nav.notifications' },
    ],
  },
  {
    titleKey: 'nav.groupConfig',
    items: [
      { to: '/agents', icon: 'robot', labelKey: 'nav.agents' },
      { to: '/sandboxes', icon: 'terminal', labelKey: 'nav.sandboxes' },
      { to: '/integrations', icon: 'connector', labelKey: 'nav.integrations' },
      { to: '/triggers', icon: 'trigger', labelKey: 'nav.triggers' },
      { to: '/settings', icon: 'settings', labelKey: 'nav.settings' },
    ],
  },
]

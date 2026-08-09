<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'
import type { AgentOrg } from '@/lib/api'
import {
  UNGROUPED_ID,
  buildOrgTreeRows,
  type OrgTreeRow,
} from '@/lib/agentOrg'

const props = defineProps<{
  org: AgentOrg
  agentNames: string[]
  activeName: string
  collapsed: boolean
}>()

const emit = defineEmits<{
  (e: 'select-agent', name: string): void
  (e: 'rename-agent', name: string): void
  (e: 'remove-from-group', name: string, groupId: string): void
  (e: 'open-manage', agentName?: string): void
  (e: 'create-root-group'): void
  (e: 'create-child-group', parentId: string): void
  (e: 'rename-group', groupId: string): void
  (e: 'delete-group', groupId: string): void
  (e: 'export-group', groupId: string): void
  (e: 'import-group', groupId: string): void
  (e: 'move-group', groupId: string, newParentId: string): void
  (e: 'move-agent', agentName: string, sourceGroupId: string, targetGroupId: string): void
  (e: 'toggle-collapsed'): void
}>()

const { t } = useI18n()

const collapsedNodes = ref<Set<string>>(new Set())
const dragOverKey = ref('')
const dragging = ref<{ kind: 'group' | 'agent'; id: string; sourceGroupId?: string } | null>(null)

type CtxState =
  | {
      open: true
      kind: 'group'
      x: number
      y: number
      groupId: string
      groupName: string
    }
  | {
      open: true
      kind: 'agent'
      x: number
      y: number
      agentName: string
      groupId: string
    }

const ctx = ref<CtxState | null>(null)

const rows = computed(() => buildOrgTreeRows(props.org, props.agentNames, collapsedNodes.value))

watch(
  () => props.org.groups?.map((g) => g.id).join(','),
  () => {
    // Expand new roots by default; keep prior collapse state for known ids.
  },
)

function toggleNode(id: string) {
  const next = new Set(collapsedNodes.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  collapsedNodes.value = next
}

function padStyle(depth: number) {
  return { paddingLeft: `${6 + depth * 14}px` }
}

function closeCtx() {
  ctx.value = null
}

function openGroupCtx(e: MouseEvent, groupId: string, groupName: string) {
  e.preventDefault()
  e.stopPropagation()
  ctx.value = {
    open: true,
    kind: 'group',
    x: e.clientX,
    y: e.clientY,
    groupId,
    groupName,
  }
}

function openAgentCtx(e: MouseEvent, agentName: string, groupId: string) {
  e.preventDefault()
  e.stopPropagation()
  ctx.value = {
    open: true,
    kind: 'agent',
    x: e.clientX,
    y: e.clientY,
    agentName,
    groupId,
  }
}

function onRowContextMenu(e: MouseEvent, row: OrgTreeRow) {
  if (row.kind === 'group') {
    openGroupCtx(e, row.id, row.name)
    return
  }
  if (row.kind === 'agent') {
    openAgentCtx(e, row.name, row.groupId)
  }
  // ungrouped-header / blank: no-op (do not open group menu)
}

function onCtxAction(action: string) {
  if (!ctx.value) return
  const current = ctx.value
  closeCtx()
  if (current.kind === 'group') {
    const id = current.groupId
    if (action === 'newChild') emit('create-child-group', id)
    else if (action === 'export') emit('export-group', id)
    else if (action === 'import') emit('import-group', id)
    else if (action === 'rename') emit('rename-group', id)
    else if (action === 'delete') emit('delete-group', id)
    return
  }
  if (action === 'renameViaManage') {
    emit('open-manage', current.agentName)
  } else if (action === 'removeFromGroup') {
    emit('remove-from-group', current.agentName, current.groupId)
  }
}

function onDragStart(e: DragEvent, row: OrgTreeRow) {
  if (row.kind === 'group') {
    dragging.value = { kind: 'group', id: row.id }
    e.dataTransfer?.setData('text/org-group', row.id)
  } else if (row.kind === 'agent') {
    dragging.value = { kind: 'agent', id: row.name, sourceGroupId: row.groupId }
    e.dataTransfer?.setData('text/org-agent', row.name)
    e.dataTransfer?.setData('text/org-source', row.groupId)
  } else {
    e.preventDefault()
    return
  }
  e.dataTransfer!.effectAllowed = 'move'
}

function onDragEnd() {
  dragging.value = null
  dragOverKey.value = ''
}

function acceptDrop(row: OrgTreeRow): boolean {
  if (!dragging.value) return false
  if (dragging.value.kind === 'group') {
    return row.kind === 'group' || row.kind === 'ungrouped-header'
  }
  return row.kind === 'group' || row.kind === 'ungrouped-header'
}

function onDragOver(e: DragEvent, row: OrgTreeRow) {
  if (!acceptDrop(row)) return
  e.preventDefault()
  dragOverKey.value = row.key
}

function onDrop(e: DragEvent, row: OrgTreeRow) {
  e.preventDefault()
  const d = dragging.value
  dragOverKey.value = ''
  dragging.value = null
  if (!d) return
  if (d.kind === 'group') {
    if (row.kind === 'group') {
      if (row.id === d.id) return
      emit('move-group', d.id, row.id)
    } else if (row.kind === 'ungrouped-header') {
      emit('move-group', d.id, '')
    }
    return
  }
  if (d.kind === 'agent') {
    const source = d.sourceGroupId || ''
    if (row.kind === 'group') {
      if (source === row.id) return
      emit('move-agent', d.id, source === UNGROUPED_ID ? '' : source, row.id)
    } else if (row.kind === 'ungrouped-header') {
      emit('move-agent', d.id, source === UNGROUPED_ID ? '' : source, '')
    }
  }
}
</script>

<template>
  <div class="flex min-h-0 min-w-0 flex-col border-r border-line">
    <div
      class="flex shrink-0 items-center gap-1 border-b border-line px-2 py-1.5 min-h-8"
      :class="collapsed ? 'justify-center px-[3px]' : ''"
    >
      <span
        v-if="!collapsed"
        class="flex-1 truncate px-1 text-[10.5px] font-semibold uppercase tracking-wider text-txt3"
      >{{ t('pages.agentStudio.agentList.title') }}</span>
      <button
        v-if="!collapsed"
        type="button"
        data-org-manage
        class="flex h-[22px] w-[22px] shrink-0 items-center justify-center text-txt3 transition hover:bg-elevated hover:text-accent-2"
        :title="t('pages.agentStudio.org.manageTitle')"
        :aria-label="t('pages.agentStudio.org.manageTitle')"
        @click="emit('open-manage')"
      >
        <Icon name="user" :size="12" />
      </button>
      <button
        v-if="!collapsed"
        type="button"
        class="flex h-[22px] w-[22px] shrink-0 items-center justify-center text-txt3 transition hover:bg-elevated hover:text-accent-2"
        :title="t('pages.agentStudio.org.newRootGroup')"
        @click="emit('create-root-group')"
      >
        <Icon name="folder" :size="13" />
      </button>
      <button
        v-if="!collapsed"
        type="button"
        class="flex h-[22px] w-[22px] shrink-0 items-center justify-center text-txt3 transition hover:bg-elevated hover:text-accent-2"
        :title="t('pages.agentStudio.agentList.collapse')"
        @click="emit('toggle-collapsed')"
      >
        <Icon name="chevron-right" :size="14" class="rotate-180" />
      </button>
      <button
        v-if="collapsed"
        type="button"
        class="flex h-[22px] w-[22px] shrink-0 items-center justify-center text-txt3 transition hover:bg-elevated hover:text-accent-2"
        :title="t('pages.agentStudio.agentList.expand')"
        @click="emit('toggle-collapsed')"
      >
        <Icon name="chevron-right" :size="14" />
      </button>
    </div>

    <div v-if="!collapsed" class="scroll-area min-h-0 flex-1 overflow-y-auto p-1.5" @click="closeCtx">
      <div
        v-for="row in rows"
        :key="row.key"
        class="group relative flex w-full items-center gap-0.5 py-0.5 pr-1 text-left text-[12px] transition"
        :class="[
          row.kind === 'agent'
            ? activeName === row.name
              ? 'bg-accent-dim'
              : 'hover:bg-elevated'
            : 'text-txt3 hover:bg-elevated hover:text-txt2',
          dragOverKey === row.key ? 'bg-accent/15 outline outline-dashed outline-1 outline-accent-2/55 -outline-offset-1' : '',
          dragging && ((row.kind === 'group' && dragging.kind === 'group' && dragging.id === row.id) ||
            (row.kind === 'agent' && dragging.kind === 'agent' && dragging.id === row.name && dragging.sourceGroupId === row.groupId))
            ? 'opacity-40'
            : '',
        ]"
        :data-org-kind="row.kind"
        :data-org-depth="row.depth"
        draggable="false"
        @dragover="onDragOver($event, row)"
        @drop="onDrop($event, row)"
        @contextmenu="onRowContextMenu($event, row)"
      >
        <!-- group: [indent chevron+name][count] -->
        <template v-if="row.kind === 'group'">
          <div
            class="flex min-w-0 flex-1 items-center gap-0.5"
            data-org-main
            :style="padStyle(row.depth)"
          >
            <button
              type="button"
              class="flex h-4 w-4 shrink-0 items-center justify-center text-txt3"
              data-org-toggle
              @click.stop="toggleNode(row.id)"
            >
              <Icon name="chevron-right" :size="12" :class="row.collapsed ? '' : 'rotate-90'" />
            </button>
            <div
              class="flex min-w-0 flex-1 cursor-grab items-center gap-1.5 py-0.5 active:cursor-grabbing"
              draggable="true"
              @dragstart="onDragStart($event, row)"
              @dragend="onDragEnd"
              @click.stop="toggleNode(row.id)"
            >
              <Icon name="folder" :size="14" class="shrink-0 text-warn" />
              <span class="truncate font-medium text-txt2">{{ row.name }}</span>
            </div>
          </div>
          <span
            data-org-count
            class="ml-1 flex w-auto min-w-[18px] shrink-0 items-center justify-end"
          >
            <span
              class="inline-flex h-4 min-w-[18px] items-center justify-end border border-line bg-base px-1 text-[10px] font-semibold tabular-nums text-txt3"
            >{{ row.count }}</span>
          </span>
        </template>

        <!-- ungrouped: same two columns, no group context menu -->
        <template v-else-if="row.kind === 'ungrouped-header'">
          <div
            class="flex min-w-0 flex-1 items-center gap-0.5"
            data-org-main
            :style="padStyle(row.depth)"
          >
            <button
              type="button"
              class="flex h-4 w-4 shrink-0 items-center justify-center text-txt3"
              data-org-toggle
              @click.stop="toggleNode('__ungrouped__')"
            >
              <Icon name="chevron-right" :size="12" :class="row.collapsed ? '' : 'rotate-90'" />
            </button>
            <div
              class="flex min-w-0 flex-1 items-center gap-1.5 py-0.5"
              @click.stop="toggleNode('__ungrouped__')"
            >
              <Icon name="folder" :size="14" class="shrink-0 text-txt3" />
              <span class="truncate font-medium text-txt2">{{ t('pages.agentStudio.org.ungrouped') }}</span>
            </div>
          </div>
          <span
            data-org-count
            class="ml-1 flex w-auto min-w-[18px] shrink-0 items-center justify-end"
          >
            <span
              class="inline-flex h-4 min-w-[18px] items-center justify-end border border-line bg-base px-1 text-[10px] font-semibold tabular-nums text-txt3"
            >{{ row.count }}</span>
          </span>
        </template>

        <!-- agent leaf: no count badge; depth only on main -->
        <template v-else>
          <div
            class="flex min-w-0 flex-1 items-center gap-0.5"
            data-org-main
            :style="padStyle(row.depth)"
          >
            <span class="inline-block h-4 w-4 shrink-0" />
            <button
              type="button"
              class="flex min-w-0 flex-1 cursor-grab items-center gap-1.5 py-0.5 text-left active:cursor-grabbing"
              draggable="true"
              @dragstart="onDragStart($event, row)"
              @dragend="onDragEnd"
              @click="emit('select-agent', row.name)"
            >
              <Icon name="robot" :size="14" class="shrink-0 text-accent-2" />
              <span class="min-w-0 flex-1">
                <span class="flex items-center gap-1">
                  <span class="truncate text-txt">{{ row.name }}</span>
                  <span
                    v-if="row.multi"
                    class="shrink-0 border border-accent-2/35 bg-accent/15 px-1 text-[9px] font-bold uppercase tracking-wide text-accent-2"
                  >{{ t('pages.agentStudio.org.multiGroup') }}</span>
                </span>
              </span>
            </button>
          </div>
        </template>
      </div>

      <p class="mx-1 mt-2 border border-dashed border-line-strong/60 bg-white/[0.015] px-2.5 py-2 text-[11px] leading-relaxed text-txt3">
        {{ t('pages.agentStudio.org.dragHint') }}
      </p>
    </div>

    <Teleport to="body">
      <div
        v-if="ctx?.open"
        class="fixed inset-0 z-[9998]"
        data-org-ctx-backdrop
        @click="closeCtx"
        @contextmenu.prevent="closeCtx"
      />
      <div
        v-if="ctx?.open"
        class="fixed z-[9999] min-w-[180px] border border-line bg-elevated py-1 shadow-card"
        data-org-ctx-menu
        :data-org-ctx-kind="ctx.kind"
        :style="{ left: ctx.x + 'px', top: ctx.y + 'px' }"
        @click.stop
      >
        <template v-if="ctx.kind === 'group'">
          <button
            type="button"
            data-org-ctx-action="newChild"
            class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[12px] text-txt2 hover:bg-overlay hover:text-txt"
            @click="onCtxAction('newChild')"
          >
            <Icon name="plus" :size="13" class="text-txt3" />
            {{ t('pages.agentStudio.org.newChildGroup') }}
          </button>
          <button
            type="button"
            data-org-ctx-action="export"
            class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[12px] text-txt2 hover:bg-overlay hover:text-txt"
            @click="onCtxAction('export')"
          >
            <Icon name="download" :size="13" class="text-txt3" />
            {{ t('pages.agentStudio.exportImport.export') }}
          </button>
          <button
            type="button"
            data-org-ctx-action="import"
            class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[12px] text-txt2 hover:bg-overlay hover:text-txt"
            @click="onCtxAction('import')"
          >
            <Icon name="input" :size="13" class="text-txt3" />
            {{ t('pages.agentStudio.exportImport.import') }}
          </button>
          <button
            type="button"
            data-org-ctx-action="rename"
            class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[12px] text-txt2 hover:bg-overlay hover:text-txt"
            @click="onCtxAction('rename')"
          >
            <Icon name="edit" :size="13" class="text-txt3" />
            {{ t('pages.agentStudio.explorer.rename') }}
          </button>
          <button
            type="button"
            data-org-ctx-action="delete"
            class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[12px] text-txt2 hover:bg-err/10 hover:text-err"
            @click="onCtxAction('delete')"
          >
            <Icon name="close" :size="13" class="text-txt3" />
            {{ t('pages.agentStudio.explorer.delete') }}
          </button>
        </template>
        <template v-else>
          <button
            type="button"
            data-org-ctx-action="renameViaManage"
            class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[12px] text-txt2 hover:bg-overlay hover:text-txt"
            @click="onCtxAction('renameViaManage')"
          >
            <Icon name="edit" :size="13" class="text-txt3" />
            {{ t('pages.agentStudio.org.renameViaManage') }}
          </button>
          <button
            v-if="ctx.groupId !== UNGROUPED_ID"
            type="button"
            data-org-ctx-action="removeFromGroup"
            class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[12px] text-txt2 hover:bg-overlay hover:text-txt"
            @click="onCtxAction('removeFromGroup')"
          >
            <Icon name="leave" :size="13" class="text-txt3" />
            {{ t('pages.agentStudio.org.removeFromGroup') }}
          </button>
        </template>
      </div>
    </Teleport>
  </div>
</template>

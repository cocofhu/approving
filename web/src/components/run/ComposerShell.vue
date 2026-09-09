<script setup lang="ts">
withDefaults(
  defineProps<{
    /** Hide the bordered input chrome (cold path: footer only). */
    showChrome?: boolean
    showFooter?: boolean
    boxTestId?: string
    toolbarTestId?: string
    footerTestId?: string
  }>(),
  {
    showChrome: true,
    showFooter: true,
    boxTestId: 'composer-shell-box',
    toolbarTestId: 'composer-shell-toolbar',
    footerTestId: 'composer-shell-footer',
  },
)
</script>

<template>
  <div data-testid="composer-shell">
    <div
      v-if="showChrome"
      class="flex min-w-0 flex-col rounded-md border border-line bg-base"
      :data-testid="boxTestId"
    >
      <div class="min-w-0 px-2 pt-2">
        <slot name="input" />
      </div>
      <!-- flex-shrink:0 so SVG/tool buttons do not stretch with auto-grow textarea (plan g1.1 / f6) -->
      <div
        class="flex h-11 shrink-0 items-center gap-1.5 px-2 pb-2 pt-1"
        :data-testid="toolbarTestId"
      >
        <slot name="toolbar-start" />
        <div class="ml-auto flex shrink-0 items-center gap-1.5">
          <slot name="toolbar-end" />
        </div>
      </div>
    </div>
    <div
      v-if="showFooter"
      class="mt-2 flex min-w-0 flex-wrap items-center justify-between gap-2"
      :data-testid="footerTestId"
    >
      <div class="min-w-0 flex-1 basis-40 text-[11px] leading-snug text-txt3 [overflow-wrap:anywhere]">
        <slot name="hint" />
      </div>
      <div class="ml-auto flex shrink-0 flex-wrap items-center justify-end gap-2">
        <slot name="footer" />
      </div>
    </div>
  </div>
</template>

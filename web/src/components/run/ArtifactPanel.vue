<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import ArtifactList from './ArtifactList.vue'
import ArtifactPreview from './ArtifactPreview.vue'
import type { Artifact } from '@/lib/types'

const props = withDefaults(defineProps<{ artifacts: Artifact[]; scope?: 'run' | 'platform' }>(), {
  scope: 'run',
})

const emit = defineEmits<{
  deleted: [id: string]
}>()

const active = ref<Artifact | null>(null)
/** Prefer this index after parent removes a deleted artifact from the list. */
const preferNextIndex = ref<number | null>(null)

function select(a: Artifact) {
  active.value = a
}

function onDeleted(id: string) {
  const idx = props.artifacts.findIndex((a) => a.id === id)
  preferNextIndex.value = idx >= 0 ? idx : null
  emit('deleted', id)
}

onMounted(() => {
  if (!active.value && props.artifacts.length) active.value = props.artifacts[0]
})

watch(
  () => props.artifacts.map((a) => a.id).join(','),
  () => {
    if (active.value && !props.artifacts.some((a) => a.id === active.value!.id)) {
      if (!props.artifacts.length) {
        active.value = null
        preferNextIndex.value = null
        return
      }
      if (preferNextIndex.value != null) {
        const i = Math.min(preferNextIndex.value, props.artifacts.length - 1)
        active.value = props.artifacts[i]
        preferNextIndex.value = null
      } else {
        active.value = props.artifacts[0]
      }
    }
  },
)
</script>

<template>
  <div class="flex h-full">
    <div class="flex w-[40%] min-w-[180px] flex-col border-r border-line">
      <ArtifactList
        :artifacts="artifacts"
        :active-id="active?.id"
        :scope="scope"
        @select="select"
      />
    </div>
    <ArtifactPreview
      :artifact="active"
      :scope="scope"
      :artifacts="artifacts"
      :run-id="active?.runId"
      @deleted="onDeleted"
    />
  </div>
</template>

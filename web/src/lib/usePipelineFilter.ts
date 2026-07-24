import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'

export const PIPELINE_FILTER_KEYS = {
  all: 'common.pipelineFilter.all',
  noMatch: 'common.pipelineFilter.noMatch',
  searchPlaceholder: 'common.search.pipelinePlaceholder',
} as const

// usePipelineFilter exposes the currently selected pipeline (workflow id) as a
// writable value backed by the URL query param `?wf=`. Backing it by the URL
// means the choice is shared across the 运行 / 待审批 / 产物 views: switching
// between those pages keeps the same pipeline in scope, and the filter is
// shareable / restorable via the link. Empty string = 全部流水线.
export function usePipelineFilter() {
  const route = useRoute()
  const router = useRouter()

  const selected = computed<string>({
    get: () => (typeof route.query.wf === 'string' ? route.query.wf : ''),
    set: (val) => {
      const query = { ...route.query }
      if (val) query.wf = val
      else delete query.wf
      router.replace({ query })
    },
  })

  return { selected }
}

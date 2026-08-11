import { computed } from 'vue'
import type { Workflow } from '@/lib/shared/types'
import type { InputField } from '@/components/workflow/RunLaunchModal.vue'

/** Extract ask=true global variables from a workflow's input node. */
export function useWorkflowAskInputs(wf: Workflow) {
  const fields = computed<InputField[]>(() => {
    const input = wf.nodes.find((n) => n.type === 'input')
    const vars = ((input?.config?.variables as any[]) || []).filter((v) => v && v.name && v.ask)
    return vars.map((v) => ({
      key: v.name,
      desc: v.desc,
      type: v.type === 'string' ? 'text' : v.type,
      required: v.required,
      default:
        v.type === 'repos'
          ? JSON.stringify(Array.isArray(v.value) ? v.value : [])
          : v.value == null
            ? ''
            : String(v.value),
      editable: v.editable,
      options: v.options,
    }))
  })
  return { fields }
}

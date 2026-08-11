<script lang="ts">
const STRUCTURED_ARTIFACT_NAMES = new Set([
  'clarified_requirement.json',
  'research.json',
  'proposals.json',
  'proposal.json',
  'plan.json',
  'implementation_result.json',
  'test_result.json',
  'review.json',
])

export function isStructuredArtifactName(name: string): boolean {
  return STRUCTURED_ARTIFACT_NAMES.has(name)
}
</script>

<script setup lang="ts">
import { computed } from 'vue'
import PlanView from './PlanView.vue'
import ProposalSelectView from './ProposalSelectView.vue'
import ClarifiedRequirementView from './product/ClarifiedRequirementView.vue'
import ResearchView from './product/ResearchView.vue'
import TestResultView from './product/TestResultView.vue'
import ReviewView from './product/ReviewView.vue'
import ImplementationResultView from './product/ImplementationResultView.vue'

import type { Artifact } from '@/lib/shared/types'

// Shared dispatcher: given a reserved artifact file name and its parsed JSON,
// render the matching structured view. Used by both the node "产物" tab and the
// human_gate body so they share one rendering path.
const props = defineProps<{
  name: string
  doc: any
  accent?: string
  runId?: string
  artifacts?: Artifact[]
  /** Live run status for test_result screenshot error gating; omit ⇒ terminal default. */
  runStatus?: string
}>()

// proposal.json is a single accepted proposal; adapt it to the proposals card
// list (one item) and highlight it as the selected one.
const asProposals = computed(() => ({ context: props.doc?.context, proposals: props.doc ? [props.doc] : [] }))
</script>

<template>
  <ClarifiedRequirementView v-if="name === 'clarified_requirement.json'" :doc="doc" :accent="accent" />
  <PlanView v-else-if="name === 'plan.json'" :doc="doc" :accent="accent" />
  <ImplementationResultView v-else-if="name === 'implementation_result.json'" :doc="doc" :accent="accent" />
  <ResearchView v-else-if="name === 'research.json'" :doc="doc" :accent="accent" />
  <TestResultView
    v-else-if="name === 'test_result.json'"
    :doc="doc"
    :accent="accent"
    :run-id="runId"
    :artifacts="artifacts"
    :run-status="runStatus"
  />
  <ReviewView v-else-if="name === 'review.json'" :doc="doc" />
  <ProposalSelectView v-else-if="name === 'proposals.json'" :doc="doc" readonly />
  <ProposalSelectView v-else-if="name === 'proposal.json'" :doc="asProposals" :resolved-id="doc?.id" readonly />
</template>

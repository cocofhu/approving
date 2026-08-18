import { ref } from 'vue'
import { normalizeShortSha } from '@/lib/shared/serviceCommit'

/** Latest service-program short SHA from GET /api/health. Empty → hide badge. */
export const serviceCommit = ref('')

/** Apply optional `commit` from a health JSON body. Does not use VITE_GIT_COMMIT. */
export function applyHealthCommit(raw: unknown): void {
  serviceCommit.value = normalizeShortSha(raw)
}

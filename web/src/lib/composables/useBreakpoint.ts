import { ref, onMounted, onBeforeUnmount } from 'vue'

const MOBILE_QUERY = '(max-width: 767px)'

function readIsMobile(): boolean {
  if (typeof window === 'undefined') return false
  return window.matchMedia(MOBILE_QUERY).matches
}

/** Reactive mobile breakpoint (≤767px). Mutually exclusive with Tailwind md (min-width:768px). */
export function useBreakpoint() {
  const isMobile = ref(readIsMobile())
  let mql: MediaQueryList | undefined

  function sync() {
    isMobile.value = !!mql?.matches
  }

  onMounted(() => {
    mql = window.matchMedia(MOBILE_QUERY)
    sync()
    mql.addEventListener('change', sync)
  })

  onBeforeUnmount(() => {
    mql?.removeEventListener('change', sync)
  })

  return { isMobile }
}

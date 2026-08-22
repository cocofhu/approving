<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import {
  createParticleMeshController,
  prefersReducedMotion,
  type ParticleMeshController,
} from '@/lib/shared/homeParticleMesh'
import { theme } from '@/lib/shared/theme'

const hostRef = ref<HTMLElement | null>(null)
const canvasRef = ref<HTMLCanvasElement | null>(null)

let controller: ParticleMeshController | null = null
let raf = 0
let reduceMotion = false
let resizeObserver: ResizeObserver | null = null

function layoutSize(): { width: number; height: number } {
  const host = hostRef.value
  if (!host) return { width: 0, height: 0 }
  const rect = host.getBoundingClientRect()
  return { width: rect.width, height: rect.height }
}

function setupCanvas() {
  const canvas = canvasRef.value
  const host = hostRef.value
  if (!canvas || !host) return { width: 0, height: 0, dpr: 1 }

  const rect = host.getBoundingClientRect()
  const dpr = Math.min(window.devicePixelRatio || 1, 2)
  const width = Math.max(1, Math.floor(rect.width))
  const height = Math.max(1, Math.floor(rect.height))
  canvas.width = Math.max(1, Math.floor(width * dpr))
  canvas.height = Math.max(1, Math.floor(height * dpr))
  canvas.style.width = `${width}px`
  canvas.style.height = `${height}px`
  const ctx = canvas.getContext('2d')
  if (ctx) ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
  return { width, height, dpr }
}

function rebuild() {
  if (!controller) return
  const { width, height } = setupCanvas()
  if (width <= 0 || height <= 0) return
  controller.rebuild(width, height)
  if (reduceMotion) paintOnce()
}

function paintOnce() {
  const canvas = canvasRef.value
  if (!canvas || !controller) return
  const ctx = canvas.getContext('2d')
  if (!ctx) return
  const { width, height } = layoutSize()
  if (width <= 0 || height <= 0) return
  controller.paint(ctx, width, height)
}

function frame() {
  const canvas = canvasRef.value
  if (!canvas || !controller) return
  const ctx = canvas.getContext('2d')
  if (!ctx) return
  const { width, height } = layoutSize()
  if (width <= 0 || height <= 0) {
    raf = requestAnimationFrame(frame)
    return
  }
  controller.step(width, height)
  controller.paint(ctx, width, height)
  raf = requestAnimationFrame(frame)
}

function startLoop() {
  stopLoop()
  if (reduceMotion) {
    paintOnce()
    return
  }
  raf = requestAnimationFrame(frame)
}

function stopLoop() {
  if (raf) {
    cancelAnimationFrame(raf)
    raf = 0
  }
}

function onMouseMove(e: MouseEvent) {
  if (!controller || !hostRef.value) return
  const rect = hostRef.value.getBoundingClientRect()
  controller.setMouse(e.clientX - rect.left, e.clientY - rect.top)
}

function onMouseLeave() {
  if (!controller) return
  const { width, height } = layoutSize()
  controller.resetMouseToCenter(width, height)
}

function onMotionPrefChange() {
  reduceMotion = prefersReducedMotion()
  startLoop()
}

let motionMq: MediaQueryList | null = null

onMounted(() => {
  controller = createParticleMeshController()
  reduceMotion = prefersReducedMotion()
  rebuild()
  startLoop()

  const host = hostRef.value
  if (host) {
    host.addEventListener('mousemove', onMouseMove)
    host.addEventListener('mouseleave', onMouseLeave)
  }

  if (typeof ResizeObserver !== 'undefined' && hostRef.value) {
    resizeObserver = new ResizeObserver(() => rebuild())
    resizeObserver.observe(hostRef.value)
  } else {
    window.addEventListener('resize', rebuild)
  }

  if (typeof window !== 'undefined' && window.matchMedia) {
    motionMq = window.matchMedia('(prefers-reduced-motion: reduce)')
    motionMq.addEventListener('change', onMotionPrefChange)
  }
})

watch(theme, () => {
  // Theme only affects shell base color; repaint keeps particles visible on both themes.
  if (reduceMotion) paintOnce()
})

onBeforeUnmount(() => {
  stopLoop()
  const host = hostRef.value
  if (host) {
    host.removeEventListener('mousemove', onMouseMove)
    host.removeEventListener('mouseleave', onMouseLeave)
  }
  resizeObserver?.disconnect()
  resizeObserver = null
  window.removeEventListener('resize', rebuild)
  motionMq?.removeEventListener('change', onMotionPrefChange)
  motionMq = null
  controller = null
})
</script>

<template>
  <div ref="hostRef" class="home-particle-mesh" data-testid="home-particle-mesh-bg" aria-hidden="true">
    <canvas ref="canvasRef" class="home-particle-mesh__canvas" />
  </div>
</template>

<style scoped>
.home-particle-mesh {
  position: absolute;
  inset: 0;
  z-index: 0;
  overflow: hidden;
  pointer-events: none;
  background: rgb(var(--c-base));
}

.home-particle-mesh__canvas {
  position: absolute;
  inset: 0;
  display: block;
  width: 100%;
  height: 100%;
  pointer-events: none;
}
</style>

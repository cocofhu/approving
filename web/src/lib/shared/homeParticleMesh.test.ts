// @vitest-environment node
import { describe, expect, it } from 'vitest'
import {
  createParticleMeshController,
  DEFAULT_PARTICLE_MESH_CONFIG,
} from './homeParticleMesh'

describe('homeParticleMesh', () => {
  it('rebuilds particles scaled to viewport area', () => {
    const ctrl = createParticleMeshController()
    ctrl.rebuild(1200, 800)
    expect(ctrl.getDotCount()).toBeGreaterThanOrEqual(80)
    expect(ctrl.getDotCount()).toBeLessThanOrEqual(DEFAULT_PARTICLE_MESH_CONFIG.maxParticles)
    expect(ctrl.getMouse()).toEqual({ x: 600, y: 400 })
  })

  it('resets mouse to center on leave', () => {
    const ctrl = createParticleMeshController()
    ctrl.rebuild(400, 300)
    ctrl.setMouse(12, 34)
    ctrl.resetMouseToCenter(400, 300)
    expect(ctrl.getMouse()).toEqual({ x: 200, y: 150 })
  })

  it('paints without throwing on a mock canvas context', () => {
    const ctrl = createParticleMeshController()
    ctrl.rebuild(320, 240)
    const calls: string[] = []
    const ctx = {
      clearRect: () => calls.push('clear'),
      beginPath: () => calls.push('begin'),
      fillStyle: '',
      strokeStyle: '',
      lineWidth: 0,
      moveTo: () => calls.push('move'),
      lineTo: () => calls.push('line'),
      stroke: () => calls.push('stroke'),
      arc: () => calls.push('arc'),
      fill: () => calls.push('fill'),
    } as unknown as CanvasRenderingContext2D
    expect(() => ctrl.paint(ctx, 320, 240)).not.toThrow()
    expect(calls).toContain('clear')
    expect(calls).toContain('fill')
  })
})

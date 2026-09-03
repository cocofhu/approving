/** Module-level counter so concurrent MermaidDiagram instances never share render ids. */
let seq = 0

/** CSS-safe mermaid render id (no dots / special selectors). */
export function nextMermaidRenderId(gen: number): string {
  seq += 1
  return `plan-mmd-${Date.now()}-${seq}-${gen}`
}

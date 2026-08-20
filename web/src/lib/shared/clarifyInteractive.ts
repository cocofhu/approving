/** True when the node type runs a live ReAct clarify dialogue (ask_question). */
export function isClarifyInteractive(type: string | null | undefined): boolean {
  return type === 'react' || type === 'approve'
}

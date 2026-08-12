import type { TokenStatsModel } from '@/lib/shared/types'

export const MODEL_PALETTE = [
  '#7B61FF',
  '#60A5FA',
  '#34D399',
  '#FBBF24',
  '#F59E0B',
  '#22D3EE',
  '#A78BFA',
  '#FB7185',
  '#4ADE80',
  '#38BDF8',
]

const UNK = '#71717A'
const OTHER = '#A1A1AA'
const FILLED = '#34D399'

export function colorForModel(m: Pick<TokenStatsModel, 'name' | 'unknown' | 'other' | 'filled'>, idx: number): string {
  if (m.unknown) return UNK
  if (m.other || m.name === 'other') return OTHER
  if (m.filled) return FILLED
  return MODEL_PALETTE[idx % MODEL_PALETTE.length]!
}

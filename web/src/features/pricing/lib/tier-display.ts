import { toIntlLocale } from '@/i18n/languages'
import { formatCompactNumber } from '@/lib/format'

/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import type { TierCondition } from './billing-expr'

type Translate = (key: string) => string

const TIER_LABEL_KEYS: Record<string, string> = {
  '0_200k': 'Up to 200K tokens',
  '200k_plus': 'Over 200K tokens',
  base: 'Base tier',
  long_context: 'Long-context tier',
  standard: 'Standard tier',
}

const CONDITION_VAR_KEYS: Record<TierCondition['var'], string> = {
  p: 'Input',
  c: 'Output',
  len: 'Length',
}

const OP_LABELS: Record<TierCondition['op'], string> = {
  '<': '<',
  '<=': '≤',
  '>': '>',
  '>=': '≥',
}

function isChineseLanguage(language: string | undefined): boolean {
  return language?.toLowerCase().startsWith('zh') === true
}

export function formatTierLabel(label: string, t: Translate): string {
  const normalized = label.trim().toLowerCase()
  if (!normalized) return t('Default')
  return t(TIER_LABEL_KEYS[normalized] || label)
}

export function formatTierTokenHint(
  value: string | number,
  language?: string
): string {
  const n = Number(value)
  if (!Number.isFinite(n) || n === 0) return ''

  if (n >= 1000) {
    const compact = formatCompactNumber(n, toIntlLocale(language))
    return isChineseLanguage(language) ? `${compact} Token` : compact
  }
  return String(n)
}

export function formatTierConditionSummary(
  conditions: TierCondition[],
  t: Translate,
  language?: string
): string {
  return conditions
    .map((condition) => {
      const variable = t(CONDITION_VAR_KEYS[condition.var] || condition.var)
      const value = formatTierTokenHint(condition.value, language)
      return `${variable} ${OP_LABELS[condition.op] || condition.op} ${value || condition.value}`
    })
    .filter(Boolean)
    .join(' && ')
}

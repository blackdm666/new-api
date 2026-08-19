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
import { describe, expect, it } from 'vitest'

import {
  formatTierConditionSummary,
  formatTierLabel,
  formatTierTokenHint,
} from '../tier-display'

const zhTranslations: Record<string, string> = {
  Length: '上下文长度',
  'Over 200K tokens': '超过 20万 Token',
  'Up to 200K tokens': '20万 Token 以内',
}
const translateZh = (key: string) => zhTranslations[key] || key
const translateEn = (key: string) => key

describe('tier display formatting', () => {
  it('localizes known tier identifiers without changing unknown labels', () => {
    expect(formatTierLabel('0_200k', translateZh)).toBe('20万 Token 以内')
    expect(formatTierLabel('200k_plus', translateZh)).toBe('超过 20万 Token')
    expect(formatTierLabel('partner_custom', translateZh)).toBe(
      'partner_custom'
    )
  })

  it('uses natural Chinese units for token thresholds', () => {
    expect(formatTierTokenHint(200_000, 'zhCN')).toBe('20万 Token')
    expect(formatTierTokenHint(272_000, 'zhTW')).toBe('27.2萬 Token')
    expect(formatTierTokenHint(200_000, 'en')).toBe('200K')
  })

  it('formats the tier condition in the active language', () => {
    const conditions = [{ var: 'len', op: '<=', value: 200_000 }] as const

    expect(
      formatTierConditionSummary([...conditions], translateZh, 'zhCN')
    ).toBe('上下文长度 ≤ 20万 Token')
    expect(formatTierConditionSummary([...conditions], translateEn, 'en')).toBe(
      'Length ≤ 200K'
    )
  })
})

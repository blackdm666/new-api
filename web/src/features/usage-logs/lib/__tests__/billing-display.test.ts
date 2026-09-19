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
import { describe, expect, test } from 'vitest'

import {
  applyLoggedGroupRatio,
  formatRatioCompact,
  getEffectiveGroupRatio,
  getEffectiveGroupRatioInfo,
} from '../billing-display'

describe('usage log billing display helpers', () => {
  test('prefers the user-specific ratio and falls back from the -1 sentinel', () => {
    expect(
      getEffectiveGroupRatioInfo({ group_ratio: 0.3, user_group_ratio: 0.2 })
    ).toEqual({ ratio: 0.2, isUserSpecific: true })
    expect(
      getEffectiveGroupRatioInfo({ group_ratio: 0.3, user_group_ratio: -1 })
    ).toEqual({ ratio: 0.3, isUserSpecific: false })
  })

  test('uses a neutral ratio for old logs without valid ratio data', () => {
    expect(getEffectiveGroupRatio(null)).toBe(1)
    expect(getEffectiveGroupRatio({ group_ratio: Number.NaN })).toBe(1)
    expect(getEffectiveGroupRatio({ group_ratio: 0 })).toBe(0)
  })

  test('applies the logged ratio exactly once to a base price', () => {
    expect(applyLoggedGroupRatio(35, { group_ratio: 0.3 })).toBeCloseTo(10.5)
    expect(
      applyLoggedGroupRatio(35, {
        group_ratio: 0.3,
        user_group_ratio: 0.2,
      })
    ).toBeCloseTo(7)
  })

  test('formats ratios without unnecessary trailing zeroes', () => {
    expect(formatRatioCompact(0.3)).toBe('0.3')
    expect(formatRatioCompact(1)).toBe('1')
    expect(formatRatioCompact(1.25)).toBe('1.25')
    expect(formatRatioCompact(0.333333)).toBe('0.3333')
  })
})

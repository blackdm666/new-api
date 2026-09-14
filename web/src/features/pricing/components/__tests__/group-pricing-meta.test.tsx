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
import { render, screen } from '@testing-library/react'
import { describe, expect, test } from 'vitest'

import {
  formatGroupPricingRatio,
  getGroupPricingRatioHeader,
} from '../../lib/group-ratio-label'
import { GroupPricingMeta } from '../group-pricing-meta'

describe('GroupPricingMeta', () => {
  test('renders the description before the ratio as separate labels', () => {
    render(
      <GroupPricingMeta
        group='Claude Max池'
        ratio={0.1}
        description='官方价1折，优惠90%'
      />
    )

    expect(screen.getByText('0.1x')).toBeInTheDocument()
    expect(screen.getByText('官方价1折，优惠90%')).toHaveAttribute(
      'title',
      '官方价1折，优惠90%'
    )
    expect(screen.getByText('官方价1折，优惠90%').nextElementSibling).toBe(
      screen.getByText('0.1x')
    )
    expect(screen.queryByText('·')).not.toBeInTheDocument()
  })

  test('hides empty and redundant descriptions', () => {
    const { rerender } = render(
      <GroupPricingMeta group='Grok' ratio={0.3} description='grok' />
    )

    expect(screen.queryByText(/^grok$/i)).not.toBeInTheDocument()

    rerender(<GroupPricingMeta group='Grok' ratio={0.3} description='  ' />)
    expect(screen.queryByText(/^grok$/i)).not.toBeInTheDocument()
  })

  test('formats Chinese discounts while preserving ratios at and above one', () => {
    expect(formatGroupPricingRatio(0.2, 'zhCN')).toBe('2折')
    expect(formatGroupPricingRatio(0.3, 'zhCN')).toBe('3折')
    expect(formatGroupPricingRatio(0.5, 'zhTW')).toBe('5折')
    expect(formatGroupPricingRatio(0.7, 'zhCN')).toBe('7折')
    expect(formatGroupPricingRatio(0, 'zhCN')).toBe('0折')
    expect(formatGroupPricingRatio(1, 'zhCN')).toBe('1x')
    expect(formatGroupPricingRatio(1.2, 'zhCN')).toBe('1.2x')
    expect(formatGroupPricingRatio(0.2, 'en')).toBe('0.2x')
  })

  test('uses the discount header only when every visible Chinese group is discounted', () => {
    expect(getGroupPricingRatioHeader([0.2, 0.3], 'zhCN', '倍率')).toBe(
      '优惠折扣'
    )
    expect(getGroupPricingRatioHeader([0.2, 0.3], 'zhTW', '倍率')).toBe(
      '優惠折扣'
    )
    expect(getGroupPricingRatioHeader([0.2], 'zh-TW', '倍率')).toBe('優惠折扣')
    expect(getGroupPricingRatioHeader([0.2, 1], 'zhCN', '倍率')).toBe('倍率')
    expect(getGroupPricingRatioHeader([1.2], 'zhCN', '倍率')).toBe('倍率')
    expect(getGroupPricingRatioHeader([0.2], 'en', 'Ratio')).toBe('Ratio')
  })
})

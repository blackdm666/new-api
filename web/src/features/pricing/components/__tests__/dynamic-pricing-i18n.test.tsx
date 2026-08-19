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
import i18next from 'i18next'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import zh from '@/i18n/locales/zh.json'

import { DynamicPricingBreakdown } from '../dynamic-pricing-breakdown'

const tieredExpression =
  'len <= 200000 ? tier("0_200k", p * 8.75 + c * 70) : tier("200k_plus", p * 17.5 + c * 105)'

describe('dynamic pricing translations', () => {
  beforeEach(async () => {
    i18next.addResourceBundle('zhCN', 'translation', zh.translation, true, true)
    await i18next.changeLanguage('zhCN')
  })

  afterEach(async () => {
    await i18next.changeLanguage('en')
  })

  it('shows localized tier labels and thresholds without exposing raw ids', () => {
    render(<DynamicPricingBreakdown billingExpr={tieredExpression} />)

    expect(screen.getAllByText('20万 Token 以内')).toHaveLength(2)
    expect(screen.getAllByText('超过 20万 Token')).toHaveLength(2)
    expect(screen.getAllByText('上下文长度 ≤ 20万 Token')).toHaveLength(2)
    expect(screen.queryByText('0_200k')).not.toBeInTheDocument()
    expect(screen.queryByText('200k_plus')).not.toBeInTheDocument()
    expect(screen.queryByText('Length ≤ 200K')).not.toBeInTheDocument()
  })
})

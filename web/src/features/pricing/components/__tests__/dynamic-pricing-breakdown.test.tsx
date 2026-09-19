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
import { beforeEach, describe, expect, test } from 'vitest'

import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { DynamicPricingBreakdown } from '../dynamic-pricing-breakdown'

const billingExpr =
  'len < 272000 ? tier("Standard", p * 5 + c * 30 + cr * 0.5 + cc * 6.25) : ' +
  'tier("Long", p * 10 + c * 45 + cr * 1 + cc * 12.5)'

describe('DynamicPricingBreakdown', () => {
  beforeEach(() => {
    useSystemConfigStore.getState().setConfig({
      currency: {
        ...DEFAULT_CURRENCY_CONFIG,
        quotaDisplayType: 'CNY',
        usdExchangeRate: 7,
      },
    })
  })

  test('shows every tier price after applying the supplied group ratio', () => {
    render(
      <DynamicPricingBreakdown
        billingExpr={billingExpr}
        groupRatioMultiplier={0.1}
      />
    )

    for (const expectedPrice of [
      '¥3.5000',
      '¥21.0000',
      '¥0.3500',
      '¥4.3750',
      '¥7.0000',
      '¥31.5000',
      '¥0.7000',
      '¥8.7500',
    ]) {
      expect(screen.getAllByText(expectedPrice)).not.toHaveLength(0)
    }

    expect(screen.queryByText('¥35.0000')).not.toBeInTheDocument()
    expect(screen.queryByText('¥210.0000')).not.toBeInTheDocument()
  })
})

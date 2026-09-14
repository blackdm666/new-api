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

import zh from '@/i18n/locales/zh.json'
import { resolveBillingCurrencyConversion } from '@/lib/currency'
import { DEFAULT_CURRENCY_CONFIG } from '@/stores/system-config-store'

import {
  pricingDisplayToUSD,
  pricingDisplayToModelRatio,
  pricingUSDToDisplay,
} from '../model-pricing-currency'

describe('model pricing currency conversion', () => {
  test('uses the concise Chinese per-second label', () => {
    expect(zh.translation['Per-second']).toBe('按秒')
  })

  test('round-trips CNY editor values through the configured USD rate', () => {
    const currency = resolveBillingCurrencyConversion({
      ...DEFAULT_CURRENCY_CONFIG,
      quotaDisplayType: 'CNY',
      usdExchangeRate: 7,
    })

    expect(currency).toEqual({ symbol: '¥', label: 'CNY', exchangeRate: 7 })
    expect(pricingUSDToDisplay('0.2', currency)).toBe('1.4')
    expect(pricingDisplayToUSD('1.4', currency)).toBe('0.2')
    expect(pricingDisplayToModelRatio('14', currency)).toBe('1')
  })

  test('keeps production-style CNY values unchanged when rate is one', () => {
    const currency = resolveBillingCurrencyConversion({
      ...DEFAULT_CURRENCY_CONFIG,
      quotaDisplayType: 'CNY',
      usdExchangeRate: 1,
    })

    expect(pricingUSDToDisplay('0.2', currency)).toBe('0.2')
    expect(pricingDisplayToUSD('0.2', currency)).toBe('0.2')
  })

  test('uses USD for monetary editors when quota display is tokens', () => {
    const currency = resolveBillingCurrencyConversion({
      ...DEFAULT_CURRENCY_CONFIG,
      quotaDisplayType: 'TOKENS',
    })

    expect(currency).toEqual({ symbol: '$', label: 'USD', exchangeRate: 1 })
  })

  test('uses the custom symbol and exchange rate', () => {
    const currency = resolveBillingCurrencyConversion({
      ...DEFAULT_CURRENCY_CONFIG,
      quotaDisplayType: 'CUSTOM',
      customCurrencySymbol: '€',
      customCurrencyExchangeRate: 0.9,
    })

    expect(currency).toEqual({ symbol: '€', label: '€', exchangeRate: 0.9 })
    expect(pricingUSDToDisplay('10', currency)).toBe('9')
    expect(pricingDisplayToUSD('9', currency)).toBe('10')
  })
})

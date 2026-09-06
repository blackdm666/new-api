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
  applyPriceSyncSelections,
  pricingOptions,
} from '@/features/model-pricing/pricing'

import { getSyncPriceLines } from '../upstream-ratio-sync-helpers'

describe('upstream fixed-price billing unit sync', () => {
  const current = () =>
    pricingOptions({
      ModelPrice: '{"video-model":0.08}',
      ModelRatio: '{"untouched":2}',
      BillingMode: '{"video-model":"per_second"}',
      BillingExpr: '{"video-model":"tier(\\"old\\", p)"}',
    })

  test('persists a selected fixed price and unit together, removing stale token and expression fields', () => {
    const result = applyPriceSyncSelections(current(), {
      'video-model': {
        model_price: 0,
        billing_mode: 'per_request',
        model_ratio: 99,
      },
    })
    expect(JSON.parse(result.ModelPrice)).toEqual({ 'video-model': 0 })
    expect(JSON.parse(result.ModelRatio)).toEqual({ untouched: 2 })
    expect(JSON.parse(result['billing_setting.billing_mode'])).toEqual({
      'video-model': 'per_request',
    })
    expect(JSON.parse(result['billing_setting.billing_expr'])).toEqual({})
  })

  test('preserves an existing explicit unit when a legacy upstream omits it', () => {
    const result = applyPriceSyncSelections(current(), {
      'video-model': { model_price: 0.1 },
    })
    expect(JSON.parse(result.ModelPrice)['video-model']).toBe(0.1)
    expect(
      JSON.parse(result['billing_setting.billing_mode'])['video-model']
    ).toBe('per_second')
  })

  test('replacing fixed pricing with token pricing clears the old price and unit', () => {
    const result = applyPriceSyncSelections(current(), {
      'video-model': { model_ratio: 2 },
    })
    expect(JSON.parse(result.ModelPrice)).toEqual({})
    expect(JSON.parse(result['billing_setting.billing_expr'])).toEqual({})
    expect(
      JSON.parse(result['billing_setting.billing_mode'])['video-model']
    ).toBe('ratio')
    expect(JSON.parse(result.ModelRatio)['video-model']).toBe(2)
  })

  test('an expression selection replaces fixed fields and preserves the exact expression', () => {
    const expression = 'tier("premium", p + c)'
    const result = applyPriceSyncSelections(current(), {
      'video-model': { billing_mode: 'tiered_expr', billing_expr: expression },
    })
    expect(JSON.parse(result.ModelPrice)).toEqual({})
    expect(
      JSON.parse(result['billing_setting.billing_mode'])['video-model']
    ).toBe('tiered_expr')
    expect(
      JSON.parse(result['billing_setting.billing_expr'])['video-model']
    ).toBe(expression)
  })

  test.each(['', '   ', '1e-324', '-1', 'NaN', 'Infinity', '1usd'])(
    'rejects upstream price %j without changing the original options',
    (model_price) => {
      const options = current()
      expect(() =>
        applyPriceSyncSelections(options, { 'video-model': { model_price } })
      ).toThrow()
      expect(JSON.parse(options.ModelPrice)['video-model']).toBe(0.08)
    }
  )

  test('the price preview identifies seconds for an explicitly per-second source', () => {
    expect(
      getSyncPriceLines(
        { model_price: 0.1, billing_mode: 'per_second' },
        (key) => key
      )[0].label
    ).toBe('Per-second')
  })
})

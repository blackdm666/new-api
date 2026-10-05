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

import { PAYMENT_TYPES } from '../constants'
import {
  dispatchSelectedPayment,
  getDiscountRateForAmount,
  getPaymentMethodDisplayName,
  isAntomPayment,
  isStripePayment,
  isWaffoPayment,
  isWaffoPancakePayment,
  mergePresetAmounts,
} from './payment'

describe('payment type classification', () => {
  test('keeps Waffo and Waffo Pancake on their dedicated flows', () => {
    expect(isWaffoPayment(PAYMENT_TYPES.WAFFO)).toBe(true)
    expect(isWaffoPayment(PAYMENT_TYPES.WAFFO_PANCAKE)).toBe(false)
    expect(isWaffoPancakePayment(PAYMENT_TYPES.WAFFO_PANCAKE)).toBe(true)
    expect(isWaffoPancakePayment(PAYMENT_TYPES.WAFFO)).toBe(false)
    expect(isStripePayment(PAYMENT_TYPES.STRIPE)).toBe(true)
    expect(isAntomPayment(PAYMENT_TYPES.ANTOM)).toBe(true)
  })
})

describe('payment method display name', () => {
  test('translates the default Antom name but preserves a custom admin name', () => {
    const translate = (key: string) => `translated:${key}`

    expect(
      getPaymentMethodDisplayName(
        { name: 'Global Wallet Payment', type: PAYMENT_TYPES.ANTOM },
        translate
      )
    ).toBe('translated:Global Wallet Payment')
    expect(
      getPaymentMethodDisplayName(
        { name: '88API Global Pay', type: PAYMENT_TYPES.ANTOM },
        translate
      )
    ).toBe('88API Global Pay')
  })
})

describe('amount discount tiers', () => {
  test('applies the highest reached threshold to larger recharge amounts', () => {
    const discounts = { 1000: 0.95, 5000: 0.9 }

    expect(getDiscountRateForAmount(999, discounts)).toBe(1)
    expect(getDiscountRateForAmount(1000, discounts)).toBe(0.95)
    expect(getDiscountRateForAmount(10000, discounts)).toBe(0.9)
  })

  test('uses threshold discounts for preset amounts', () => {
    expect(mergePresetAmounts([500, 1000, 10000], { 1000: 0.95 })).toEqual([
      { value: 500, discount: 1 },
      { value: 1000, discount: 0.95 },
      { value: 10000, discount: 0.95 },
    ])
  })
})

describe('payment dispatch', () => {
  test('keeps the selected Waffo method index through confirmation', async () => {
    const calls: string[] = []
    const success = await dispatchSelectedPayment(
      { name: 'Waffo Card', type: PAYMENT_TYPES.WAFFO },
      120,
      3,
      {
        regular: async () => {
          calls.push('regular')
          return false
        },
        waffo: async (amount, index) => {
          calls.push(`waffo:${amount}:${index}`)
          return true
        },
        waffoPancake: async () => {
          calls.push('pancake')
          return false
        },
      }
    )

    expect(success).toBe(true)
    expect(calls).toEqual(['waffo:120:3'])
  })

  test('does not create a Waffo order without a selected method index', async () => {
    let called = false
    const success = await dispatchSelectedPayment(
      { name: 'Waffo Card', type: PAYMENT_TYPES.WAFFO },
      120,
      null,
      {
        regular: async () => false,
        waffo: async () => {
          called = true
          return true
        },
        waffoPancake: async () => false,
      }
    )

    expect(success).toBe(false)
    expect(called).toBe(false)
  })

  test('routes Antom through the regular processor with its dedicated type', async () => {
    const calls: string[] = []
    const success = await dispatchSelectedPayment(
      { name: 'Global Wallet Payment', type: PAYMENT_TYPES.ANTOM },
      10,
      null,
      {
        regular: async (amount, paymentType) => {
          calls.push(`${paymentType}:${amount}`)
          return true
        },
        waffo: async () => false,
        waffoPancake: async () => false,
      }
    )

    expect(success).toBe(true)
    expect(calls).toEqual(['antom:10'])
  })
})

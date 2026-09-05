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
  applyResolutionSelection,
  buildSyncedPricingOptions,
  deleteResolutionField,
  isSelectableUpstreamValue,
  type PricingOptionMaps,
  resolveResolutionSelection,
  type RatioDifferenceEntry,
} from '../upstream-ratio-sync-helpers'

const sourceName = 'upstream(1)'
const entry = (value: number | string): RatioDifferenceEntry => ({
  current: null,
  upstreams: { [sourceName]: value },
  confidence: { [sourceName]: true },
})

const currentPricing = (): PricingOptionMaps => ({
  ModelRatio: {},
  CompletionRatio: {},
  CacheRatio: {},
  CreateCacheRatio: {},
  ImageRatio: {},
  AudioRatio: {},
  AudioCompletionRatio: {},
  ModelPrice: {},
  'billing_setting.billing_mode': {},
  'billing_setting.billing_expr': {},
})

describe('upstream fixed-price billing unit sync', () => {
  const differences = {
    'video-model': {
      model_price: entry(0.08),
      billing_mode: entry('per_second'),
    },
  }

  test('folds a fixed billing mode into the model-price selection', () => {
    expect(
      resolveResolutionSelection(differences, {
        model: 'video-model',
        ratioType: 'billing_mode',
        value: 'per_second',
        sourceName,
      })
    ).toMatchObject({
      ratioType: 'model_price',
      value: 0.08,
    })
  })

  test('persists price and unit together and removes stale expressions', () => {
    expect(
      applyResolutionSelection(
        {
          'video-model': {
            billing_mode: 'tiered_expr',
            billing_expr: 'tier("old", p)',
          },
        },
        differences,
        {
          model: 'video-model',
          ratioType: 'model_price',
          value: 0.08,
          sourceName,
        }
      )
    ).toEqual({
      'video-model': {
        model_price: 0.08,
        billing_mode: 'per_second',
      },
    })
  })

  test('preserves an existing fixed unit when a legacy upstream omits it', () => {
    const legacyDifferences = {
      'video-model': {
        model_price: entry(0.1),
      },
    }

    expect(
      applyResolutionSelection(
        {
          'video-model': {
            model_price: 0.08,
            billing_mode: 'per_second',
          },
        },
        legacyDifferences,
        {
          model: 'video-model',
          ratioType: 'model_price',
          value: 0.1,
          sourceName,
        }
      )
    ).toEqual({
      'video-model': {
        model_price: 0.1,
        billing_mode: 'per_second',
      },
    })
  })

  test('clears fixed and tiered state when switching to token ratios', () => {
    const ratioDifferences = {
      'video-model': {
        model_ratio: entry(2),
      },
    }

    expect(
      applyResolutionSelection(
        {
          'video-model': {
            model_price: 0.08,
            billing_mode: 'per_second',
            billing_expr: 'tier("old", p)',
          },
        },
        ratioDifferences,
        {
          model: 'video-model',
          ratioType: 'model_ratio',
          value: 2,
          sourceName,
        }
      )
    ).toEqual({
      'video-model': {
        model_ratio: 2,
      },
    })
  })

  test('removes persisted fixed state when the staged selection switches to ratios', () => {
    const current = currentPricing()
    current.ModelPrice['video-model'] = 0.08
    current['billing_setting.billing_mode']['video-model'] = 'per_second'
    current['billing_setting.billing_expr']['video-model'] = 'tier("old", p)'

    const result = buildSyncedPricingOptions(current, {
      'video-model': { model_ratio: 2 },
    })

    expect(result.ModelPrice['video-model']).toBeUndefined()
    expect(
      result['billing_setting.billing_mode']['video-model']
    ).toBeUndefined()
    expect(
      result['billing_setting.billing_expr']['video-model']
    ).toBeUndefined()
    expect(result.ModelRatio['video-model']).toBe(2)
  })

  test('preserves a persisted fixed unit for a legacy upstream price', () => {
    const current = currentPricing()
    current.ModelPrice['video-model'] = 0.08
    current['billing_setting.billing_mode']['video-model'] = 'per_second'

    const result = buildSyncedPricingOptions(current, {
      'video-model': { model_price: 0.1 },
    })

    expect(result.ModelPrice['video-model']).toBe(0.1)
    expect(result['billing_setting.billing_mode']['video-model']).toBe(
      'per_second'
    )
  })

  test('clears persisted tiered state when applying a legacy upstream price', () => {
    const current = currentPricing()
    current['billing_setting.billing_mode']['video-model'] = 'tiered_expr'
    current['billing_setting.billing_expr']['video-model'] = 'tier("old", p)'

    const result = buildSyncedPricingOptions(current, {
      'video-model': { model_price: 0.1 },
    })

    expect(result.ModelPrice['video-model']).toBe(0.1)
    expect(
      result['billing_setting.billing_mode']['video-model']
    ).toBeUndefined()
    expect(
      result['billing_setting.billing_expr']['video-model']
    ).toBeUndefined()
  })

  test('removes the hidden fixed unit when a folded price selection is canceled', () => {
    const selected = applyResolutionSelection({}, differences, {
      model: 'video-model',
      ratioType: 'model_price',
      value: 0.08,
      sourceName,
    })

    expect(
      deleteResolutionField(selected, 'video-model', 'model_price')
    ).toEqual({})
  })

  test('rejects empty and underflow numeric values from upstream sync', () => {
    expect(isSelectableUpstreamValue('', 'model_price')).toBe(false)
    expect(isSelectableUpstreamValue('   ', 'model_ratio')).toBe(false)
    expect(isSelectableUpstreamValue('1e-324', 'model_price')).toBe(false)
    expect(isSelectableUpstreamValue('0', 'model_price')).toBe(true)

    const current = currentPricing()
    current.ModelPrice['video-model'] = 0.08
    const result = buildSyncedPricingOptions(current, {
      'video-model': { model_price: '' },
    })
    expect(result.ModelPrice['video-model']).toBe(0.08)
  })

  test('a tiered selection replaces earlier staged fixed fields atomically', () => {
    const fixedDifferences = {
      'video-model': {
        model_price: entry(0.08),
        billing_mode: entry('per_second'),
      },
    }
    const tieredDifferences = {
      'video-model': {
        billing_expr: entry('tier("premium", p + c)'),
      },
    }

    const fixed = applyResolutionSelection({}, fixedDifferences, {
      model: 'video-model',
      ratioType: 'model_price',
      value: 0.08,
      sourceName,
    })
    const tiered = applyResolutionSelection(fixed, tieredDifferences, {
      model: 'video-model',
      ratioType: 'billing_expr',
      value: 'tier("premium", p + c)',
      sourceName,
    })

    expect(tiered).toEqual({
      'video-model': {
        billing_mode: 'tiered_expr',
        billing_expr: 'tier("premium", p + c)',
      },
    })
  })
})

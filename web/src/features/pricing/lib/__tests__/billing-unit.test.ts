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
  EMPTY_LANE_ENABLED,
  EMPTY_LANE_PRICES,
  buildPreviewRows,
} from '@/features/system-settings/models/model-pricing-core'
import {
  buildModelSnapshots,
  getPriceSummary,
} from '@/features/system-settings/models/model-pricing-snapshots'

import { QUOTA_TYPES } from '../../constants'
import type { PricingModel } from '../../types'
import { filterByQuotaType } from '../filters'
import { getFixedPriceUnitLabel, isPerSecondModel } from '../model-helpers'

const t = (key: string) => key

const fixedModel = (
  modelName: string,
  billingUnit?: 'request' | 'second'
): PricingModel =>
  ({
    id: 1,
    model_name: modelName,
    quota_type: 1,
    model_ratio: 0,
    completion_ratio: 0,
    model_price: 0.08,
    billing_unit: billingUnit,
    enable_groups: ['default'],
  }) as PricingModel

describe('fixed-price billing units', () => {
  test('separates per-request and per-second catalog filters', () => {
    const requestModel = fixedModel('request-model', 'request')
    const legacyModel = fixedModel('legacy-model')
    const secondModel = fixedModel('second-model', 'second')
    const models = [requestModel, legacyModel, secondModel]

    expect(filterByQuotaType(models, QUOTA_TYPES.REQUEST)).toEqual([
      requestModel,
      legacyModel,
    ])
    expect(filterByQuotaType(models, QUOTA_TYPES.SECOND)).toEqual([secondModel])
    expect(isPerSecondModel(secondModel)).toBe(true)
    expect(getFixedPriceUnitLabel(secondModel)).toBe('second')
    expect(getFixedPriceUnitLabel(legacyModel)).toBe('request')
  })

  test('round-trips explicit billing modes in pricing snapshots', () => {
    const snapshots = buildModelSnapshots({
      modelPrice: '{"request-model":0.5,"second-model":0.08}',
      modelRatio: '{}',
      cacheRatio: '{}',
      createCacheRatio: '{}',
      completionRatio: '{}',
      imageRatio: '{}',
      audioRatio: '{}',
      audioCompletionRatio: '{}',
      billingMode:
        '{"request-model":"per_request","second-model":"per_second"}',
      billingExpr: '{}',
    })

    const request = snapshots.find((item) => item.name === 'request-model')
    const second = snapshots.find((item) => item.name === 'second-model')
    if (!request || !second) throw new Error('expected pricing snapshots')
    expect(request?.billingMode).toBe('per-request')
    expect(second?.billingMode).toBe('per-second')
    expect(getPriceSummary(request, t)).toBe('$0.5 / request')
    expect(getPriceSummary(second, t)).toBe('$0.08 / second')
  })

  test('serializes the per-second editor mode explicitly', () => {
    expect(
      buildPreviewRows(
        { name: 'video-model', price: '0.08' },
        'per-second',
        '',
        '',
        '',
        EMPTY_LANE_PRICES,
        EMPTY_LANE_ENABLED,
        t
      )
    ).toEqual([
      { key: 'mode', label: 'BillingMode', value: 'per_second' },
      { key: 'price', label: 'ModelPrice', value: '0.08' },
    ])
  })
})

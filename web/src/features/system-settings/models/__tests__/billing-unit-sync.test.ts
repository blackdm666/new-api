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
  resolveResolutionSelection,
  type RatioDifferenceEntry,
} from '../upstream-ratio-sync-helpers'

const sourceName = 'upstream(1)'
const entry = (value: number | string): RatioDifferenceEntry => ({
  current: null,
  upstreams: { [sourceName]: value },
  confidence: { [sourceName]: true },
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
})

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

import type { PricingModel } from '../../types'
import { resolvePricingModelIcon } from '../model-icon'

const model = (name: string, icon?: string): PricingModel => ({
  id: 1,
  model_name: name,
  icon,
  quota_type: 0,
  model_ratio: 1,
  completion_ratio: 1,
  enable_groups: [],
})

describe('pricing model icon resolution', () => {
  test('uses the Hunyuan logo for Chinese and English model names', () => {
    expect(resolvePricingModelIcon(model('HY混元'))).toBe('Hunyuan.Color')
    expect(resolvePricingModelIcon(model('hunyuan-t1'))).toBe('Hunyuan.Color')
  })

  test('preserves an explicitly configured icon', () => {
    expect(resolvePricingModelIcon(model('HY混元', 'Tencent.Color'))).toBe(
      'Tencent.Color'
    )
  })
})

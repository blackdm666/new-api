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

import { createModelPricingSchema } from '../model-pricing-core'

const schema = createModelPricingSchema((key) => key)

describe('model pricing numeric validation', () => {
  test.each(['.', '1abc', '-1', 'NaN', 'Infinity', '1e-324'])(
    'rejects incomplete or non-finite price %s',
    (price) => {
      expect(schema.safeParse({ name: 'video-model', price }).success).toBe(
        false
      )
    }
  )

  test.each(['', '0', '0e-999', '1.', '.5', '1.25', '1e-6'])(
    'accepts empty or complete non-negative price %s',
    (price) => {
      expect(schema.safeParse({ name: 'video-model', price }).success).toBe(
        true
      )
    }
  )
})

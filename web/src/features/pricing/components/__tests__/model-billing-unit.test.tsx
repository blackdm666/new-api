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
import { describe, expect, test } from 'vitest'

import type { PricingModel } from '../../types'
import { ModelBillingModeBadge } from '../model-billing-mode-badge'

const fixedModel = (billingUnit?: 'request' | 'second') =>
  ({
    id: 1,
    model_name: 'video-model',
    quota_type: 1,
    model_ratio: 0,
    completion_ratio: 0,
    model_price: 0.08,
    billing_unit: billingUnit,
    enable_groups: ['default'],
  }) as PricingModel

describe('ModelBillingModeBadge fixed-price units', () => {
  test('labels explicit seconds separately from legacy request prices', () => {
    const { rerender } = render(
      <ModelBillingModeBadge model={fixedModel('second')} />
    )
    expect(screen.getByText('Per-second')).toBeInTheDocument()

    rerender(<ModelBillingModeBadge model={fixedModel()} />)
    expect(screen.getByText('Per Request')).toBeInTheDocument()
  })
})

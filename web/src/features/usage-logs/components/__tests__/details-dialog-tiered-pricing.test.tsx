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
import type { ReactNode } from 'react'
import { describe, expect, test, vi } from 'vitest'

import { usageLogSchema } from '../../data/schema'
import { DetailsDialog } from '../dialogs/details-dialog'

vi.mock('@/components/dialog', () => ({
  Dialog: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}))

vi.mock('@/features/pricing/components/dynamic-pricing-breakdown', () => ({
  DynamicPricingBreakdown: ({
    groupRatioMultiplier,
  }: {
    groupRatioMultiplier?: number
  }) => (
    <div data-testid='dynamic-pricing-breakdown'>
      {String(groupRatioMultiplier)}
    </div>
  ),
}))

vi.mock('@/features/pricing/hooks/use-pricing-data', () => ({
  usePricingData: () => ({ models: [] }),
}))

describe('usage log tiered price table', () => {
  test('passes the effective logged group ratio to the price table', () => {
    const log = usageLogSchema.parse({
      id: 1,
      user_id: 1,
      created_at: 1,
      type: 2,
      content: '',
      other: JSON.stringify({
        billing_mode: 'tiered_expr',
        expr_b64: btoa('tier("Standard", p * 5 + c * 30)'),
        group_ratio: 0.3,
        user_group_ratio: 0.1,
      }),
    })

    render(
      <DetailsDialog log={log} isAdmin open onOpenChange={() => undefined} />
    )

    expect(screen.getByTestId('dynamic-pricing-breakdown')).toHaveTextContent(
      '0.1'
    )
  })
})

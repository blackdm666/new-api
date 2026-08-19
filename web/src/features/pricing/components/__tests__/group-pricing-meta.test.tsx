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

import { GroupPricingMeta } from '../group-pricing-meta'

describe('GroupPricingMeta', () => {
  test('renders the ratio and configured discount description together', () => {
    render(
      <GroupPricingMeta
        group='Claude Max池'
        ratio={0.1}
        description='官方价1折，优惠90%'
      />
    )

    expect(screen.getByText('0.1x')).toBeInTheDocument()
    expect(screen.getByText('官方价1折，优惠90%')).toHaveAttribute(
      'title',
      '官方价1折，优惠90%'
    )
  })

  test('hides empty and redundant descriptions', () => {
    const { rerender } = render(
      <GroupPricingMeta group='Grok' ratio={0.3} description='grok' />
    )

    expect(screen.queryByText(/^grok$/i)).not.toBeInTheDocument()

    rerender(<GroupPricingMeta group='Grok' ratio={0.3} description='  ' />)
    expect(screen.queryByText(/^grok$/i)).not.toBeInTheDocument()
  })
})

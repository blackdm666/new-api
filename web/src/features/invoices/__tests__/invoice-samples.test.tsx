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
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, test } from 'vitest'

import { InvoiceSampleDialog } from '../components/invoice-samples'
import type { InvoiceRegion } from '../types'

function SampleDialogHarness(props: { initialRegion: InvoiceRegion }) {
  const [region, setRegion] = useState(props.initialRegion)
  return (
    <InvoiceSampleDialog
      open
      onOpenChange={() => undefined}
      region={region}
      onRegionChange={setRegion}
    />
  )
}

describe('invoice sample dialog', () => {
  test('opening on the mainland tab shows the general e-invoice sample image', () => {
    render(<SampleDialogHarness initialRegion='mainland' />)

    const image = screen.getByRole('img', {
      name: 'VAT electronic ordinary invoice (Mainland China)',
    })
    expect(image).toBeVisible()
    expect(image.getAttribute('src')).toMatch(/mainland/)
  })

  test('switching to the Hong Kong tab shows the INVOICE sample image', async () => {
    const user = userEvent.setup()
    render(<SampleDialogHarness initialRegion='mainland' />)

    await user.click(screen.getByRole('tab', { name: 'Hong Kong' }))

    const image = screen.getByRole('img', { name: 'INVOICE (Hong Kong)' })
    expect(image).toBeVisible()
    expect(image.getAttribute('src')).toMatch(/hong-kong/)
  })
})

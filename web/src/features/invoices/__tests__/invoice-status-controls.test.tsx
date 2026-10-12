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
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { InvoiceStatusControls } from '../components/invoice-status-controls'
import { INVOICE_STATUS, type InvoiceRequest } from '../types'

const pendingInvoice = {
  id: 42,
  user_id: 7,
  company_name: 'Example Technology Co., Ltd.',
  tax_number: '91310000MA1FL8XQ3B',
  total_money: 888,
  tax_fee_cents: 2664,
  tax_rate_basis_points: 300,
  tax_fee_refunded_time: 0,
  status: INVOICE_STATUS.PENDING,
} as InvoiceRequest

function renderControls(fileCount: number) {
  const onUpdated = vi.fn().mockResolvedValue(undefined)
  render(
    <InvoiceStatusControls
      invoice={pendingInvoice}
      fileCount={fileCount}
      onUpdated={onUpdated}
    />
  )
  return { onUpdated }
}

async function chooseStatus(
  user: ReturnType<typeof userEvent.setup>,
  name: string
) {
  await user.click(screen.getByRole('combobox', { name: 'Application status' }))
  await user.click(await screen.findByRole('option', { name }))
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('invoice status controls', () => {
  test('without uploaded files the issued status cannot be chosen and a hint explains why', async () => {
    const user = userEvent.setup()
    renderControls(0)

    expect(
      screen.getByText(
        'Upload at least one invoice file before marking the application as issued.'
      )
    ).toBeVisible()
    await user.click(
      screen.getByRole('combobox', { name: 'Application status' })
    )

    expect(
      await screen.findByRole('option', { name: 'Issued' })
    ).toHaveAttribute('aria-disabled', 'true')
    expect(screen.getByRole('button', { name: 'Update status' })).toBeDisabled()
  })

  test('marking as issued asks for confirmation before updating the status', async () => {
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true, data: null } })
    const user = userEvent.setup()
    const { onUpdated } = renderControls(2)

    await chooseStatus(user, 'Issued')
    await user.click(screen.getByRole('button', { name: 'Update status' }))

    const dialog = await screen.findByRole('alertdialog')
    expect(
      within(dialog).getByText('Mark this invoice application as issued?')
    ).toBeVisible()
    expect(within(dialog).getByText(pendingInvoice.company_name)).toBeVisible()
    expect(put).not.toHaveBeenCalled()

    await user.click(
      within(dialog).getByRole('button', { name: 'Confirm issue' })
    )

    expect(put).toHaveBeenCalledWith(
      '/api/invoice/admin/requests/42/status',
      { status: INVOICE_STATUS.ISSUED, rejection_reason: '' },
      expect.anything()
    )
    expect(onUpdated).toHaveBeenCalledOnce()
  })

  test('rejecting shows the reason and the tax fee refund before confirming', async () => {
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true, data: null } })
    const user = userEvent.setup()
    renderControls(0)

    await chooseStatus(user, 'Rejected')
    expect(screen.getByRole('button', { name: 'Update status' })).toBeDisabled()
    await user.type(
      screen.getByLabelText('Rejection reason'),
      'Tax number does not match the company'
    )
    await user.click(screen.getByRole('button', { name: 'Update status' }))

    const dialog = await screen.findByRole('alertdialog')
    expect(
      within(dialog).getByText('Tax number does not match the company')
    ).toBeVisible()
    expect(within(dialog).getByText(/¥26\.64/)).toBeVisible()
    expect(put).not.toHaveBeenCalled()

    await user.click(
      within(dialog).getByRole('button', { name: 'Confirm rejection' })
    )

    expect(put).toHaveBeenCalledWith(
      '/api/invoice/admin/requests/42/status',
      {
        status: INVOICE_STATUS.REJECTED,
        rejection_reason: 'Tax number does not match the company',
      },
      expect.anything()
    )
  })
})

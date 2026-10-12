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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { InvoiceDetailPage } from '../components/invoice-detail-page'
import {
  INVOICE_STATUS,
  type InvoiceRequest,
  type InvoiceRequestDetail,
} from '../types'

const { navigate } = vi.hoisted(() => ({ navigate: vi.fn() }))
vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  useNavigate: () => navigate,
  useRouter: () => ({ history: { back: vi.fn() } }),
  useCanGoBack: () => false,
}))

function detail(overrides: Partial<InvoiceRequest>): InvoiceRequestDetail {
  return {
    invoice: {
      id: 42,
      user_id: 7,
      company_name: 'Example Technology Co., Ltd.',
      tax_number: '91310000MA1FL8XQ3B',
      bank_name: '',
      bank_account: '',
      company_address: '',
      company_phone: '',
      remark: '',
      topup_order_ids: '1',
      total_money: 600,
      total_money_cents: 60_000,
      tax_rate_basis_points: 300,
      tax_fee_cents: 1_800,
      tax_fee_quota: 0,
      tax_fee_refunded_time: 0,
      status: INVOICE_STATUS.PENDING,
      rejection_reason: '',
      issued_time: 0,
      redacted_time: 0,
      expiry_warning_time: 0,
      expires_at: 0,
      created_time: 1_760_000_000,
      updated_time: 1_760_000_000,
      ...overrides,
    },
    orders: [],
    files: [],
    events: [],
  }
}

function renderDetail(invoiceId = 42) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <InvoiceDetailPage invoiceId={invoiceId} admin={false} />
    </QueryClientProvider>
  )
}

function mockDetail(value: InvoiceRequestDetail) {
  return vi
    .spyOn(api, 'get')
    .mockResolvedValue({ data: { success: true, data: value } })
}

afterEach(() => {
  navigate.mockReset()
  vi.restoreAllMocks()
})

describe('invoice detail tax fee refund', () => {
  test('a refunded application shows the refunded tax fee', async () => {
    mockDetail(
      detail({
        status: INVOICE_STATUS.REJECTED,
        tax_fee_refunded_time: 1_760_086_400,
      })
    )

    renderDetail()

    expect(
      await screen.findByText(/^Tax fee refunded: ¥18\.00 \(/)
    ).toBeVisible()
  })

  test('a withdrawn application without a refund timestamp does not claim a refund', async () => {
    mockDetail(detail({ status: INVOICE_STATUS.WITHDRAWN }))

    renderDetail()

    expect(await screen.findByText('Tax fee paid')).toBeVisible()
    expect(screen.queryByText(/Tax fee refunded/)).not.toBeInTheDocument()
  })

  test('the withdrawal confirmation explains that the tax fee is refunded', async () => {
    mockDetail(detail({ status: INVOICE_STATUS.PENDING }))
    const user = userEvent.setup()

    renderDetail()
    await user.click(
      await screen.findByRole('button', { name: 'Withdraw application' })
    )

    const dialog = await screen.findByRole('alertdialog')
    expect(
      within(dialog).getByText(
        'The tax fee of ¥18.00 will be refunded to your balance.'
      )
    ).toBeVisible()
  })
})

describe('invoice detail loading states', () => {
  test('an invalid id shows not found without requesting or spinning', () => {
    const get = vi.spyOn(api, 'get')

    renderDetail(0)

    expect(screen.getByText('Invoice not found')).toBeVisible()
    expect(get).not.toHaveBeenCalled()
  })

  test('a failed request shows an error with retry instead of not found', async () => {
    const get = vi
      .spyOn(api, 'get')
      .mockResolvedValueOnce({
        data: { success: false, message: 'Service unavailable' },
      })
      .mockResolvedValueOnce({
        data: { success: true, data: detail({}) },
      })
    const user = userEvent.setup()

    renderDetail()

    expect(
      await screen.findByText('Failed to load the invoice application')
    ).toBeVisible()
    expect(screen.getByText('Service unavailable')).toBeVisible()
    expect(screen.queryByText('Invoice not found')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Retry' }))

    expect(
      await screen.findByText('Example Technology Co., Ltd.')
    ).toBeVisible()
    expect(get).toHaveBeenCalledTimes(2)
  })
})

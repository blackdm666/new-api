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

import { InvoiceTitlesDialog } from '../components/invoice-titles-dialog'
import type { InvoiceTitle } from '../types'

const hkTitle: InvoiceTitle = {
  id: 7,
  region: 'hk',
  company_name: 'Example HK Ltd',
  tax_number: '',
  bank_name: '',
  bank_account: '',
  company_address: '',
  company_phone: '',
  is_default: false,
  created_time: 1_760_000_000,
  updated_time: 1_760_000_000,
}

function success(data: unknown) {
  return Promise.resolve({ data: { success: true, data } })
}

function renderDialog() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <InvoiceTitlesDialog open onOpenChange={vi.fn()} />
    </QueryClientProvider>
  )
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('saved invoice titles dialog', () => {
  test('setting a default resends the title with only Hong Kong fields', async () => {
    vi.spyOn(api, 'get').mockImplementation(() => success([hkTitle]))
    const put = vi
      .spyOn(api, 'put')
      .mockImplementation(() => success({ ...hkTitle, is_default: true }))
    const user = userEvent.setup()

    renderDialog()

    const row = (await screen.findByText('Example HK Ltd')).closest('li')
    expect(row).not.toBeNull()
    expect(
      within(row as HTMLElement).getByText('INVOICE (Hong Kong)')
    ).toBeVisible()
    await user.click(
      within(row as HTMLElement).getByRole('button', { name: /Set as default/ })
    )

    expect(put).toHaveBeenCalledWith(
      '/api/invoice/titles/7',
      {
        region: 'hk',
        company_name: 'Example HK Ltd',
        tax_number: '',
        bank_name: '',
        bank_account: '',
        company_address: '',
        company_phone: '',
        is_default: true,
      },
      expect.anything()
    )
  })

  test('a new mainland title is not saved without a tax number', async () => {
    vi.spyOn(api, 'get').mockImplementation(() => success([]))
    const post = vi.spyOn(api, 'post')
    const user = userEvent.setup()

    renderDialog()

    await user.click(await screen.findByRole('button', { name: /Add title/ }))
    await user.type(screen.getByLabelText(/Invoice title/), 'Example Ltd')
    await user.click(screen.getByRole('button', { name: 'Save' }))

    expect(await screen.findByText('Tax number is required')).toBeVisible()
    expect(post).not.toHaveBeenCalled()
  })
})

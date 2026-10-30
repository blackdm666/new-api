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
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { AdminInvoices, Invoices } from '../index'
import { invoiceListSearchSchema } from '../lib/invoice-list-search'
import {
  INVOICE_STATUS,
  type InvoiceFile,
  type InvoiceRequest,
  type InvoiceStatus,
} from '../types'

function invoice(id: number, status: InvoiceStatus): InvoiceRequest {
  return {
    id,
    user_id: 7,
    username: 'buyer',
    company_name: `Example Company ${id}`,
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
    status,
    rejection_reason: '',
    issued_time: 0,
    redacted_time: 0,
    expiry_warning_time: 0,
    expires_at: 0,
    created_time: 1_760_000_000,
    updated_time: 1_760_000_000,
  }
}

function invoiceFile(id: number): InvoiceFile {
  return {
    id,
    invoice_request_id: 5,
    uploader_id: 1,
    file_name: `invoice-${id}.pdf`,
    mime_type: 'application/pdf',
    size: 3,
    storage_type: 'local',
    sha256: 'hash',
    created_time: 1_760_000_000,
  }
}

const config = {
  minimum_amount_cents: 50_000,
  tax_rate_basis_points: 300,
  available_balance_cents: 0,
  response_working_days: 3,
  email: 'billing@example.com',
  email_bound: true,
  pending_expiry_days: 30,
  file_allowed_exts: ['pdf'],
  file_max_count: 5,
  file_max_size: 5_242_880,
}

function success(data: unknown) {
  return Promise.resolve({ data: { success: true, data } })
}

async function renderInvoiceRoutes(
  initialUrl: string,
  options: { items: InvoiceRequest[]; files?: InvoiceFile[] }
) {
  const get = vi.spyOn(api, 'get').mockImplementation((url) => {
    if (/^\/api\/invoice\/(admin\/)?requests\?/.test(url)) {
      return success({ items: options.items, total: 25 })
    }
    if (url.startsWith('/api/invoice/config')) return success(config)
    if (url.startsWith('/api/invoice/requests/5?')) {
      return success({
        invoice: options.items[0],
        orders: [],
        files: options.files ?? [],
        events: [],
      })
    }
    if (url.startsWith('/api/invoice/requests/5/files/')) {
      return Promise.resolve({ data: new Blob(['pdf']) })
    }
    return Promise.reject(new Error(`unexpected GET ${url}`))
  })
  const root = createRootRoute()
  const authenticated = createRoute({
    getParentRoute: () => root,
    id: '_authenticated',
  })
  const routes = [
    createRoute({
      getParentRoute: () => authenticated,
      path: 'admin-invoices/',
      validateSearch: invoiceListSearchSchema,
      component: AdminInvoices,
    }),
    createRoute({
      getParentRoute: () => authenticated,
      path: 'admin-invoices/$id',
      component: () => <p>Admin invoice detail</p>,
    }),
    createRoute({
      getParentRoute: () => authenticated,
      path: 'invoices/',
      validateSearch: invoiceListSearchSchema,
      component: Invoices,
    }),
    createRoute({
      getParentRoute: () => authenticated,
      path: 'invoices/$id',
      component: () => <p>User invoice detail</p>,
    }),
  ]
  const router = createRouter({
    routeTree: root.addChildren([authenticated.addChildren(routes)]),
    history: createMemoryHistory({ initialEntries: [initialUrl] }),
  })
  await router.load()
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return { get, router }
}

function listRequestUrls(get: { mock: { calls: unknown[][] } }) {
  return get.mock.calls
    .map(([url]) => String(url))
    .filter((url) => url.includes('/requests?'))
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('admin invoice list URL state', () => {
  test('without search params the admin list requests pending applications', async () => {
    const { get } = await renderInvoiceRoutes('/admin-invoices/', {
      items: [invoice(1, INVOICE_STATUS.PENDING)],
    })

    expect(await screen.findByText('Example Company 1')).toBeVisible()
    expect(listRequestUrls(get)).toEqual([
      expect.stringMatching(
        /^\/api\/invoice\/admin\/requests\?p=1&page_size=10&status=1&_=/
      ),
    ])
  })

  test('changing the status filter is written to the URL and returns to page 1 with a single request', async () => {
    const { get, router } = await renderInvoiceRoutes(
      '/admin-invoices/?status=3&page=2',
      { items: [invoice(1, INVOICE_STATUS.REJECTED)] }
    )
    const user = userEvent.setup()
    await screen.findByText('Example Company 1')
    expect(listRequestUrls(get)).toEqual([
      expect.stringMatching(/\?p=2&page_size=10&status=3&_=/),
    ])

    await user.click(
      screen.getByRole('combobox', { name: 'Filter by application status' })
    )
    await user.click(
      await screen.findByRole('option', { name: 'All statuses' })
    )

    await waitFor(() =>
      expect(router.state.location.search).toEqual({ status: 'all' })
    )
    await waitFor(() => expect(listRequestUrls(get)).toHaveLength(2))
    expect(listRequestUrls(get)[1]).toMatch(/\?p=1&page_size=10&_=/)
  })

  test('returning from the detail page restores the filtered list', async () => {
    const { router } = await renderInvoiceRoutes(
      '/admin-invoices/?status=3&page=2',
      { items: [invoice(1, INVOICE_STATUS.REJECTED)] }
    )
    const user = userEvent.setup()

    await user.click(await screen.findByText('Example Company 1'))
    expect(await screen.findByText('Admin invoice detail')).toBeVisible()

    router.history.back()

    expect(await screen.findByText('Example Company 1')).toBeVisible()
    expect(router.state.location.search).toEqual({ status: 3, page: 2 })
    expect(
      screen.getByRole('combobox', { name: 'Filter by application status' })
    ).toHaveTextContent('Rejected')
  })

  test('opening the list without a keyword clears the search box instead of restoring the old keyword', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    try {
      const { router } = await renderInvoiceRoutes(
        '/admin-invoices/?keyword=Example',
        { items: [invoice(1, INVOICE_STATUS.PENDING)] }
      )
      const searchBox = await screen.findByRole('textbox', {
        name: 'Search by order number, username, invoice title, or tax number',
      })
      expect(searchBox).toHaveValue('Example')

      await act(async () => {
        router.history.push('/admin-invoices/')
      })
      await waitFor(() => expect(searchBox).toHaveValue(''))
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1000)
      })

      expect(router.state.location.search).toEqual({})
      expect(searchBox).toHaveValue('')
    } finally {
      vi.useRealTimers()
    }
  })
})

describe('user invoice list quick download', () => {
  test('an issued application with one file downloads it directly', async () => {
    vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:invoice')
    vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => undefined)
    const click = vi
      .spyOn(HTMLAnchorElement.prototype, 'click')
      .mockImplementation(() => undefined)
    const { get, router } = await renderInvoiceRoutes('/invoices/', {
      items: [invoice(5, INVOICE_STATUS.ISSUED)],
      files: [invoiceFile(9)],
    })
    const user = userEvent.setup()

    await user.click(
      await screen.findByRole('button', {
        name: 'Download invoice for application #5',
      })
    )

    await waitFor(() => expect(click).toHaveBeenCalledOnce())
    expect(get).toHaveBeenCalledWith(
      '/api/invoice/requests/5/files/9',
      expect.objectContaining({ responseType: 'blob' })
    )
    expect(router.state.location.pathname).toBe('/invoices/')
  })

  test('an issued application with several files opens the files section of the detail page', async () => {
    const { router } = await renderInvoiceRoutes('/invoices/', {
      items: [invoice(5, INVOICE_STATUS.ISSUED)],
      files: [invoiceFile(9), invoiceFile(10)],
    })
    const user = userEvent.setup()

    await user.click(
      await screen.findByRole('button', {
        name: 'Download invoice for application #5',
      })
    )

    expect(await screen.findByText('User invoice detail')).toBeVisible()
    expect(router.state.location.pathname).toBe('/invoices/5')
    expect(router.state.location.hash).toBe('invoice-files')
  })

  test('the banner states the configured response time in working days', async () => {
    await renderInvoiceRoutes('/invoices/', {
      items: [invoice(5, INVOICE_STATUS.PENDING)],
    })

    expect(
      await screen.findByText(/We respond within 3 working days/)
    ).toBeVisible()
    expect(
      screen.queryByRole('button', {
        name: 'Download invoice for application #5',
      })
    ).not.toBeInTheDocument()
  })
})

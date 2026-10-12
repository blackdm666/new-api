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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ComponentProps } from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { InvoiceRequestDialog } from '../components/invoice-request-dialog'
import type { EligibleTopUpOrder, InvoiceConfig } from '../types'

vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  Link: ({ to, ...rest }: ComponentProps<'a'> & { to: string }) => (
    <a href={to} {...rest} />
  ),
}))

const config: InvoiceConfig = {
  minimum_amount_cents: 50_000,
  tax_rate_basis_points: 300,
  available_balance_cents: 100_000,
  issue_day: 15,
  email: 'billing@example.com',
  email_bound: true,
  pending_expiry_days: 30,
  file_allowed_exts: ['pdf'],
  file_max_count: 5,
  file_max_size: 5 * 1024 * 1024,
}

const order: EligibleTopUpOrder = {
  id: 11,
  trade_no: 'PAY-20261012-001',
  payment_method: 'alipay',
  amount: 600,
  money: 600,
  status: 'success',
  create_time: 1_760_000_000,
}

type Deferred = {
  promise: Promise<unknown>
  resolve: (value: unknown) => void
}

function deferred(): Deferred {
  let resolve: (value: unknown) => void = () => undefined
  const promise = new Promise<unknown>((done) => {
    resolve = done
  })
  return { promise, resolve }
}

function success(data: unknown) {
  return { data: { success: true, data } }
}

function mockInvoiceRequests(responses: {
  config: () => Promise<unknown>
  orders: () => Promise<unknown>
}) {
  return vi.spyOn(api, 'get').mockImplementation((url) => {
    if (url.startsWith('/api/invoice/config')) return responses.config()
    if (url.startsWith('/api/invoice/eligible_orders')) {
      return responses.orders()
    }
    return Promise.reject(new Error(`unexpected GET ${url}`))
  })
}

function renderDialog() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <InvoiceRequestDialog open onOpenChange={vi.fn()} onCreated={vi.fn()} />
    </QueryClientProvider>
  )
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('invoice request dialog', () => {
  test('shows a loading state instead of empty orders until config and orders both load', async () => {
    const configResponse = deferred()
    const ordersResponse = deferred()
    mockInvoiceRequests({
      config: () => configResponse.promise,
      orders: () => ordersResponse.promise,
    })

    renderDialog()

    expect(screen.getByText('Loading invoice information...')).toBeVisible()
    expect(
      screen.queryByText('No eligible paid orders')
    ).not.toBeInTheDocument()

    configResponse.resolve(success(config))
    expect(
      await screen.findByText('Loading invoice information...')
    ).toBeVisible()

    ordersResponse.resolve(success([order]))
    expect(await screen.findByText(order.trade_no)).toBeVisible()
    expect(
      screen.getByText('The invoice will be sent to billing@example.com')
    ).toBeVisible()
    expect(screen.getByText(/on day 15 of each month/)).toBeVisible()
    expect(
      screen.queryByText('Loading invoice information...')
    ).not.toBeInTheDocument()
  })

  test('shows an error state when loading fails and recovers after retry', async () => {
    let configAttempts = 0
    mockInvoiceRequests({
      config: () => {
        configAttempts += 1
        if (configAttempts === 1) {
          return Promise.resolve({
            data: { success: false, message: 'Invoice service unavailable' },
          })
        }
        return Promise.resolve(success(config))
      },
      orders: () => Promise.resolve(success([order])),
    })
    const user = userEvent.setup()

    renderDialog()

    expect(
      await screen.findByText('Failed to load invoice information')
    ).toBeVisible()
    expect(screen.getByText('Invoice service unavailable')).toBeVisible()
    expect(
      screen.queryByText('No eligible paid orders')
    ).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Retry' }))

    expect(await screen.findByText(order.trade_no)).toBeVisible()
    expect(configAttempts).toBe(2)
  })

  test('blocks submission and links to the binding page when no email is bound', async () => {
    mockInvoiceRequests({
      config: () =>
        Promise.resolve(success({ ...config, email: '', email_bound: false })),
      orders: () => Promise.resolve(success([order])),
    })
    const post = vi.spyOn(api, 'post')
    const user = userEvent.setup()

    renderDialog()

    expect(
      await screen.findByText('No email address is bound to your account')
    ).toBeVisible()
    expect(screen.getByText('Bind Email').closest('a')).toHaveAttribute(
      'href',
      '/security'
    )

    await user.click(screen.getByRole('checkbox'))
    await user.type(screen.getByLabelText(/Invoice title/), 'Example Ltd')
    await user.type(screen.getByLabelText(/Tax number/), '91310000MA1FL8XQ3B')

    expect(
      screen.getByRole('button', { name: 'Submit invoice application' })
    ).toBeDisabled()
    expect(post).not.toHaveBeenCalled()
  })

  test('submits the tax number trimmed and in upper case', async () => {
    mockInvoiceRequests({
      config: () => Promise.resolve(success(config)),
      orders: () => Promise.resolve(success([order])),
    })
    const post = vi
      .spyOn(api, 'post')
      .mockResolvedValue(success({ invoice: { id: 1 } }))
    const user = userEvent.setup()

    renderDialog()

    await user.click(await screen.findByRole('checkbox'))
    await user.type(screen.getByLabelText(/Invoice title/), 'Example Ltd')
    await user.type(
      screen.getByLabelText(/Tax number/),
      '  91310000ma1fl8xq3b '
    )
    await user.click(
      screen.getByRole('button', { name: 'Submit invoice application' })
    )

    expect(post).toHaveBeenCalledWith(
      '/api/invoice/requests',
      expect.objectContaining({
        tax_number: '91310000MA1FL8XQ3B',
        topup_order_ids: [order.id],
      }),
      expect.anything()
    )
  })
})

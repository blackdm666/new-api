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
import { cleanup, render, screen, within } from '@testing-library/react'
import { createInstance } from 'i18next'
import type { ReactNode } from 'react'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { CommissionRecords, InviteeStats } from '../index'
import { AFFILIATE_COMMISSION_STATUS } from '../types'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

async function renderWithProviders(node: ReactNode) {
  const i18n = createInstance()
  await i18n.use(initReactI18next).init({
    lng: 'en',
    resources: { en: { translation: {} } },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={i18n}>{node}</I18nextProvider>
    </QueryClientProvider>
  )
}

function mockPage(items: unknown[]) {
  return vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { items, total: items.length } },
  })
}

describe('promoter commission records', () => {
  test('shows only the masked invitee, commission, status and time', async () => {
    mockPage([
      {
        id: 7,
        invitee_username: 'ab***',
        commission_cents: 500,
        status: AFFILIATE_COMMISSION_STATUS.APPROVED,
        approved_time: 1_786_700_900,
        created_time: 1_786_700_800,
        // Fields an older server might still send must never be rendered.
        trade_no: 'ORDER-SECRET-1',
        topup_amount_cents: 10_000,
        invitee_id: 4242,
      },
    ])

    await renderWithProviders(<CommissionRecords />)

    const row = (await screen.findByText('ab***')).closest('tr')
    expect(row).not.toBeNull()
    expect(within(row as HTMLElement).getByText('Approved')).toBeVisible()
    const headers = screen
      .getAllByRole('columnheader')
      .map((header) => header.textContent)
    expect(headers).toEqual([
      'Invited user',
      'Commission',
      'Status',
      'Created at',
    ])
    expect(document.body.textContent).not.toMatch(/ORDER-SECRET-1|4242|100\.00/)
  })
})

describe('promoter invitee list', () => {
  test('hides top-up columns and explains capped invitees when a limit is set', async () => {
    mockPage([
      {
        username: 'ca***',
        created_at: 1_786_700_000,
        is_new: false,
        commission_cents: 1_000,
        last_commission_time: 1_786_700_500,
        topup_amount_cents: 20_000,
      },
    ])

    await renderWithProviders(<InviteeStats inviteeTopUpLimit={10} />)

    expect(await screen.findByText('ca***')).toBeVisible()
    const headers = screen
      .getAllByRole('columnheader')
      .map((header) => header.textContent)
    expect(headers).toEqual([
      'User',
      'Registered at',
      'Generated commission',
      'Last commission',
    ])
    expect(
      screen.getByText(/reached the 10-top-up commission limit are not listed/)
    ).toBeVisible()
    expect(document.body.textContent).not.toMatch(/200\.00/)
  })

  test('shows no capped-invitee note when the top-up limit is unlimited', async () => {
    mockPage([])

    await renderWithProviders(<InviteeStats inviteeTopUpLimit={0} />)

    expect(await screen.findByText('No invitees yet')).toBeVisible()
    expect(screen.queryByText(/commission limit are not listed/)).toBeNull()
  })
})

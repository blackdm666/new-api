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
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { AdminAffiliatePage } from '../admin'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

async function openTab(tab: string) {
  const i18n = createInstance()
  await i18n
    .use(initReactI18next)
    .init({ lng: 'en', resources: { en: { translation: {} } } })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    const path = String(url).split('?')[0]
    let items: unknown[] = []
    if (path.endsWith('/commissions')) {
      items = [
        {
          id: 1,
          inviter_id: 42,
          invitee_id: 43,
          inviter_username: 'promoter',
          invitee_username: 'buyer',
          inviter_remark: 'promoter review note',
          invitee_remark: 'buyer review note',
          status: 1,
          trade_no: 'test',
          topup_amount_cents: 100,
          rate_basis_points: 500,
          commission_cents: 5,
          tier_name: '初级推广',
          created_time: 1,
        },
      ]
    }
    if (path.endsWith('/upgrade-candidates')) {
      items = [
        {
          inviter_id: 42,
          username: 'promoter',
          remark: 'upgrade review note',
          current_group: 'default',
          effective_invitee_count: 50,
          threshold: 50,
          effective_top_up_amount_cents: 100,
          top_up_amount_threshold_cents: 200000,
          eligible_by_invitees: true,
          eligible_by_top_up_amount: false,
          next_group: '高级推广',
          next_rate_basis_points: 1000,
        },
      ]
    }
    if (path.endsWith('/notification-failures')) {
      items = [
        {
          id: 1,
          inviter_id: 42,
          inviter_username: 'promoter',
          inviter_remark: 'notice review note',
          threshold: 50,
          effective_invitee_count: 50,
          top_up_amount_threshold_cents: 200000,
          effective_top_up_amount_cents: 100,
          attempt_count: 1,
          last_error: 'delivery failed',
          next_attempt_time: 0,
          dead_letter_time: 0,
        },
      ]
    }
    if (path.endsWith('/transfers')) {
      items = [
        {
          id: 1,
          user_id: 42,
          username: 'promoter',
          remark: 'transfer review note',
          amount_cents: 100,
          amount_quota: 500000,
          balance_cents_before: 200,
          balance_cents_after: 100,
          quota_before: 0,
          quota_after: 500000,
          created_time: 1,
        },
      ]
    }
    return { data: { success: true, data: { items, total: items.length } } }
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <AdminAffiliatePage />
      </QueryClientProvider>
    </I18nextProvider>
  )
  const user = userEvent.setup()
  await user.click(screen.getByRole('tab', { name: tab }))
  return { user, client }
}

test('commission review shows both administrator remarks without changing review controls', async () => {
  const { client } = await openTab('Commission review')
  expect(await screen.findByText('Remark: promoter review note')).toBeVisible()
  expect(screen.getByText('Remark: buyer review note')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Approve' })).toBeEnabled()
  client.clear()
})

test('upgrade review and notification failures show promoter remarks', async () => {
  const { client } = await openTab('Upgrade review')
  expect(await screen.findByText('Remark: upgrade review note')).toBeVisible()
  expect(await screen.findByText('Remark: notice review note')).toBeVisible()
  client.clear()
})

test('settlement balance transfers also show the user remark', async () => {
  const { user, client } = await openTab('Commission settlement')
  await user.click(
    screen.getByRole('tab', { name: 'Balance transfer records' })
  )
  expect(await screen.findByText('Remark: transfer review note')).toBeVisible()
  client.clear()
})

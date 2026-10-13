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
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { AccountsTab } from '../components/accounts-tab'
import type { CyberGuardAccountRow } from '../types'

function accountRow(id: number, status: string): CyberGuardAccountRow {
  return {
    account_id: id,
    account_name: `openai-${id}`,
    hits_cyber: 2,
    hits_bio: 0,
    hits_usage: 0,
    users: 1,
    last_hit_at: 1_760_000_000,
    status,
    schedulable: status === 'unknown' ? null : status === 'active',
    error_message: '',
  }
}

function statusCellOf(accountName: string) {
  const statusColumn = screen
    .getAllByRole('columnheader')
    .findIndex((header) => header.textContent === 'Account status')
  const row = screen.getByText(accountName).closest('tr')
  expect(row).not.toBeNull()
  return within(row as HTMLElement).getAllByRole('cell')[statusColumn]
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('account statistics', () => {
  test('sub2api account statuses show translated labels and unknown values stay raw', async () => {
    const rows = [
      accountRow(1, 'active'),
      accountRow(2, 'disabled'),
      accountRow(3, 'error'),
      accountRow(4, 'unknown'),
      accountRow(5, 'paused'),
    ]
    vi.spyOn(api, 'get').mockImplementation((url) => {
      if (url === '/api/cyber-guard/accounts') {
        return Promise.resolve({ data: { success: true, data: rows } })
      }
      return Promise.reject(new Error(`unexpected GET ${url}`))
    })
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })

    render(
      <QueryClientProvider client={client}>
        <AccountsTab />
      </QueryClientProvider>
    )

    await screen.findByText('openai-1')
    expect(statusCellOf('openai-1')).toHaveTextContent('Enabled')
    expect(statusCellOf('openai-2')).toHaveTextContent('Disabled')
    expect(statusCellOf('openai-3')).toHaveTextContent('Error')
    expect(statusCellOf('openai-4')).toHaveTextContent('Unknown')
    expect(statusCellOf('openai-5')).toHaveTextContent('paused')
  })
})

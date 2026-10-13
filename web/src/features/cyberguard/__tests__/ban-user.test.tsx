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
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { UsersTab } from '../components/users-tab'
import type { CyberGuardUserRow } from '../types'

function userRow(
  id: number,
  overrides: Partial<CyberGuardUserRow> = {}
): CyberGuardUserRow {
  return {
    user_id: id,
    username: `user${id}`,
    email: 'u***@example.invalid',
    current_group: 'default',
    role: 1,
    units_7d: 1,
    units_30d: 1,
    hits_30d: 3,
    last_hit_at: 1_760_000_000,
    ban: null,
    is_reseller: false,
    ...overrides,
  }
}

function renderUsers(rows: CyberGuardUserRow[]) {
  const get = vi.spyOn(api, 'get').mockImplementation((url) => {
    if (url === '/api/cyber-guard/users') {
      return Promise.resolve({
        data: {
          success: true,
          data: { page: 1, page_size: 10, total: rows.length, items: rows },
        },
      })
    }
    return Promise.reject(new Error(`unexpected GET ${url}`))
  })
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      success: true,
      data: { id: 3, case_id: 'CG-3', username: 'user5950' },
    },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <UsersTab onOpenEvents={() => undefined} />
    </QueryClientProvider>
  )
  return { get, post }
}

function rowOf(text: string) {
  const row = screen.getByText(text).closest('tr')
  expect(row).not.toBeNull()
  return within(row as HTMLElement)
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('manual ban from the users tab', () => {
  test('banning until manual unban posts hours 0 and the typed reason', async () => {
    const user = userEvent.setup()
    const { post } = renderUsers([userRow(5950)])

    await screen.findByText('user5950')
    await user.click(rowOf('user5950').getByRole('button', { name: 'Ban' }))
    const dialog = await screen.findByRole('dialog')
    const hours = within(dialog).getByLabelText('Ban duration (hours)')
    await user.clear(hours)
    await user.type(hours, '0')
    await user.type(within(dialog).getByLabelText('Reason'), 'repeat cyber')
    await user.click(screen.getByRole('button', { name: 'Ban user' }))

    await waitFor(() =>
      expect(post).toHaveBeenCalledWith('/api/cyber-guard/bans', {
        user_id: 5950,
        hours: 0,
        reason: 'repeat cyber',
      })
    )
  })

  test('a duration above one year shows a field error and sends nothing', async () => {
    const user = userEvent.setup()
    const { post } = renderUsers([userRow(5950)])

    await screen.findByText('user5950')
    await user.click(rowOf('user5950').getByRole('button', { name: 'Ban' }))
    const dialog = await screen.findByRole('dialog')
    const hours = within(dialog).getByLabelText('Ban duration (hours)')
    await user.clear(hours)
    await user.type(hours, '9000')
    await user.click(screen.getByRole('button', { name: 'Ban user' }))

    expect(
      await within(dialog).findByText('Enter whole hours from 0 to 8760.')
    ).toBeVisible()
    expect(hours).toHaveAttribute('aria-invalid', 'true')
    expect(post).not.toHaveBeenCalled()
  })

  test('administrators cannot be banned and banned users offer unban instead', async () => {
    renderUsers([
      userRow(1, { role: 100, username: 'root' }),
      userRow(2752, {
        current_group: 'banned',
        ban: {
          id: 9,
          case_id: 'CG-9',
          status: 'active',
          source: 'adopted',
          expires_at: 0,
        },
      }),
    ])

    await screen.findByText('root')
    expect(rowOf('root').getByRole('button', { name: 'Ban' })).toBeDisabled()
    const banned = rowOf('user2752')
    expect(banned.getByRole('button', { name: 'Unban' })).toBeVisible()
    expect(banned.queryByRole('button', { name: 'Ban' })).toBeNull()
  })

  test('an exempt user cannot be banned and the disabled button says why', async () => {
    renderUsers([userRow(4872, { is_exempt: true }), userRow(5950)])

    await screen.findByText('user4872')
    const exemptBan = rowOf('user4872').getByRole('button', { name: 'Ban' })
    expect(exemptBan).toBeDisabled()
    expect(exemptBan).toHaveAttribute('title', 'Exempt users cannot be banned')
    expect(rowOf('user4872').getByText('Exempt')).toBeVisible()
    expect(rowOf('user5950').getByRole('button', { name: 'Ban' })).toBeEnabled()
  })

  test('an exempt user with an open proposal cannot have it applied', async () => {
    renderUsers([
      userRow(4872, {
        is_exempt: true,
        ban: {
          id: 4,
          case_id: 'CG-4',
          status: 'observed',
          source: 'auto',
          expires_at: 0,
        },
      }),
    ])

    await screen.findByText('user4872')
    expect(
      rowOf('user4872').getByRole('button', { name: 'Apply ban' })
    ).toBeDisabled()
  })

  test('searching sends the keyword and restarts at page 1', async () => {
    const user = userEvent.setup()
    const { get } = renderUsers([userRow(5950)])

    await screen.findByText('user5950')
    await user.type(screen.getByLabelText('Search users'), ' 4872 ')
    await user.click(screen.getByRole('button', { name: 'Search' }))

    await waitFor(() =>
      expect(get).toHaveBeenLastCalledWith('/api/cyber-guard/users', {
        params: { keyword: '4872', status: 'all', p: 1, page_size: 10 },
      })
    )
  })
})

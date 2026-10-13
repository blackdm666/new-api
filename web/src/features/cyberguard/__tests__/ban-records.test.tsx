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

import { BansTab } from '../components/bans-tab'
import type { CyberGuardBanRow } from '../types'

function banRow(
  id: number,
  status: CyberGuardBanRow['status']
): CyberGuardBanRow {
  return {
    id,
    case_id: `CG-${id}`,
    user_id: 100 + id,
    username: `user${id}`,
    original_group: 'default',
    restore_group: 'default',
    ban_group: 'banned',
    source: status === 'observed' ? 'auto' : 'manual',
    status,
    tier_index: -1,
    ban_hours: 72,
    units_summary: '7天2个/30天2个',
    reason: '手动封禁',
    started_at: 1_760_000_000,
    expires_at: status === 'active' ? 1_760_259_200 : 0,
    ended_at: 0,
    end_reason: '',
    end_note: '',
    ended_by: 0,
    created_by: 1,
    email_state: 'queued',
    email_delivery_key: `cyberguard-ban:${id}`,
    email_subject: '',
    email_error: '',
    created_at: 1_760_000_000,
    updated_at: 1_760_000_000,
    current_group: status === 'active' ? 'banned' : 'default',
    email_delivery_state: status === 'active' ? 'delivered' : '',
  }
}

function ok(data: unknown) {
  return Promise.resolve({ data: { success: true, data } })
}

function renderBans(rows: CyberGuardBanRow[]) {
  vi.spyOn(api, 'get').mockImplementation((url) => {
    if (url === '/api/cyber-guard/bans') {
      return ok({ page: 1, page_size: 10, total: rows.length, items: rows })
    }
    if (url === '/api/cyber-guard/settings') {
      return ok({ ban_group: 'banned' })
    }
    return Promise.reject(new Error(`unexpected GET ${url}`))
  })
  const post = vi.spyOn(api, 'post')
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <BansTab onClearUser={() => undefined} />
    </QueryClientProvider>
  )
  return post
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('ban records', () => {
  test('an active ban offers unban and resend, a proposal offers apply and dismiss', async () => {
    renderBans([banRow(7, 'active'), banRow(8, 'observed')])

    const active = (await screen.findByText('CG-7')).closest('tr')
    const proposal = screen.getByText('CG-8').closest('tr')
    expect(active).not.toBeNull()
    expect(proposal).not.toBeNull()
    const activeRow = within(active as HTMLElement)
    const proposalRow = within(proposal as HTMLElement)

    expect(activeRow.getByRole('button', { name: 'Unban' })).toBeVisible()
    expect(activeRow.getByRole('button', { name: 'Resend' })).toBeVisible()
    expect(activeRow.queryByRole('button', { name: 'Apply ban' })).toBeNull()
    expect(proposalRow.getByRole('button', { name: 'Apply ban' })).toBeVisible()
    expect(proposalRow.getByRole('button', { name: 'Dismiss' })).toBeVisible()
    expect(proposalRow.queryByRole('button', { name: 'Unban' })).toBeNull()
  })

  test('confirming an unban posts the optional note to that case and closes the dialog', async () => {
    const user = userEvent.setup()
    const post = renderBans([banRow(7, 'active')])
    post.mockImplementation(() =>
      ok({
        ban: { ...banRow(7, 'ended'), end_reason: 'manual_unban' },
        from_group: 'banned',
        to_group: 'default',
        group_changed: true,
      })
    )

    await user.click(await screen.findByRole('button', { name: 'Unban' }))
    const dialog = await screen.findByRole('alertdialog')
    await user.type(
      within(dialog).getByLabelText('Note (optional)'),
      'appeal accepted'
    )
    await user.click(within(dialog).getByRole('button', { name: 'Unban' }))

    await waitFor(() =>
      expect(post).toHaveBeenCalledWith('/api/cyber-guard/bans/7/unban', {
        reason: 'appeal accepted',
      })
    )
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  })

  test('a failed promote keeps the dialog open for another attempt', async () => {
    const user = userEvent.setup()
    const post = renderBans([banRow(8, 'observed')])
    post.mockRejectedValue(new Error('user is already in the ban group'))

    await user.click(await screen.findByRole('button', { name: 'Apply ban' }))
    const dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: 'Apply ban' }))

    await waitFor(() =>
      expect(post).toHaveBeenCalledWith('/api/cyber-guard/bans/8/promote')
    )
    expect(screen.getByRole('alertdialog')).toBeVisible()
  })

  test('a ban whose restore group drifted shows the group before the ban', async () => {
    renderBans([
      { ...banRow(7, 'active'), restore_group: 'vip' },
      banRow(9, 'active'),
    ])

    const drifted = within(
      (await screen.findByText('CG-7')).closest('tr') as HTMLElement
    )
    const plain = within(screen.getByText('CG-9').closest('tr') as HTMLElement)

    expect(drifted.getByText('Before ban')).toBeVisible()
    expect(drifted.getByText('default')).toBeVisible()
    expect(drifted.getByText('vip')).toBeVisible()
    expect(plain.queryByText('Before ban')).toBeNull()
  })
})

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

import { SettingsTab } from '../components/settings-tab'
import type { CyberGuardSettings } from '../types'

const settings: CyberGuardSettings = {
  mode: 'observe',
  protected_channel_ids: [1, 9, 37],
  sub2api_base_url: 'http://sub2api:8080',
  sub2api_api_key_ids: [1, 6],
  poll_interval_seconds: 30,
  lookback_minutes: 15,
  count_categories: ['cyber', 'bio'],
  nosession_merge_seconds: 60,
  tiers: [
    { window_days: 7, min_units: 2, ban_hours: 72 },
    { window_days: 30, min_units: 3, ban_hours: 0 },
  ],
  ban_group: 'banned',
  exempt_user_ids: [],
  email_enabled: true,
  notify_root: true,
  has_sub2api_admin_api_key: true,
}

function renderSettings(current: CyberGuardSettings = settings) {
  vi.spyOn(api, 'get').mockImplementation((url) => {
    if (url === '/api/cyber-guard/settings') {
      return Promise.resolve({ data: { success: true, data: current } })
    }
    return Promise.reject(new Error(`unexpected GET ${url}`))
  })
  const put = vi.spyOn(api, 'put').mockImplementation((_url, body) =>
    Promise.resolve({
      data: { success: true, data: { ...current, ...(body as object) } },
    })
  )
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <SettingsTab />
    </QueryClientProvider>
  )
  return put
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('CyberGuard settings form', () => {
  test('switching to auto ban asks for confirmation before saving', async () => {
    const user = userEvent.setup()
    const put = renderSettings()

    await user.click(await screen.findByRole('radio', { name: /Auto ban/ }))
    await user.click(screen.getByRole('button', { name: 'Save settings' }))

    const dialog = await screen.findByRole('alertdialog')
    expect(put).not.toHaveBeenCalled()
    await user.click(
      within(dialog).getByRole('button', { name: 'Turn on automatic bans' })
    )

    await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
    const body = put.mock.calls[0][1] as Record<string, unknown>
    expect(body.mode).toBe('enforce')
    expect(body).not.toHaveProperty('sub2api_admin_api_key')
  })

  test('cancelling the auto-ban confirmation saves nothing', async () => {
    const user = userEvent.setup()
    const put = renderSettings()

    await user.click(await screen.findByRole('radio', { name: /Auto ban/ }))
    await user.click(screen.getByRole('button', { name: 'Save settings' }))
    const dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))

    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(put).not.toHaveBeenCalled()
  })

  test('the admin key is write-only and only a newly typed key is sent', async () => {
    const user = userEvent.setup()
    const put = renderSettings()

    const keyInput = await screen.findByLabelText(/sub2api admin API key/)
    expect(keyInput).toHaveValue('')
    expect(keyInput).toHaveAttribute('type', 'password')
    expect(screen.getByText('Saved')).toBeVisible()
    await user.type(keyInput, 'sk-admin-rotated')
    await user.click(screen.getByRole('button', { name: 'Save settings' }))

    await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
    expect(put.mock.calls[0]).toEqual([
      '/api/cyber-guard/settings',
      expect.objectContaining({
        mode: 'observe',
        sub2api_admin_api_key: 'sk-admin-rotated',
        tiers: settings.tiers,
      }),
    ])
  })

  test('an out-of-range poll interval blocks saving with a field error', async () => {
    const user = userEvent.setup()
    const put = renderSettings()

    const interval = await screen.findByLabelText('Poll interval (seconds)')
    await user.clear(interval)
    await user.type(interval, '5')
    await user.click(screen.getByRole('button', { name: 'Save settings' }))

    expect(
      await screen.findByText('Enter a whole number in the allowed range.')
    ).toBeVisible()
    expect(interval).toHaveAttribute('aria-invalid', 'true')
    expect(put).not.toHaveBeenCalled()
  })

  test('tiers can be added up to five and the last one cannot be removed', async () => {
    const user = userEvent.setup()
    renderSettings({ ...settings, tiers: [settings.tiers[0]] })

    expect(
      await screen.findByRole('button', { name: 'Remove tier 1' })
    ).toBeDisabled()
    const add = screen.getByRole('button', { name: 'Add tier' })
    for (let count = 1; count < 5; count++) await user.click(add)

    expect(screen.getByRole('button', { name: 'Remove tier 5' })).toBeEnabled()
    expect(add).toBeDisabled()
  })
})

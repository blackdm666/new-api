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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Toaster } from 'sonner'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { CyberGuardAdminPage } from '..'
import { cyberGuardSearchSchema } from '../lib/search'
import type { CyberGuardOverview } from '../types'

const overview: CyberGuardOverview = {
  mode: 'observe',
  is_master_node: false,
  last_poll: {
    at: 0,
    ok: false,
    status: '',
    error: '',
    fetched: 0,
    new_events: 0,
    cursor_end: 0,
  },
  counts: {
    hits_today: 2,
    hits_7d: 5,
    units_7d: 3,
    users_flagged_7d: 2,
    active_bans: 1,
    observed_open: 1,
    unattributed_7d: 0,
    pending: 0,
  },
  reconciliation_24h: {
    sub2api_rows: 5,
    attributed: 5,
    unattributed: 0,
    pending: 0,
  },
}

const emptyPage = { page: 1, page_size: 10, total: 0, items: [] }

async function renderPage(
  initialUrl: string,
  overviewData: CyberGuardOverview = overview
) {
  const get = vi.spyOn(api, 'get').mockImplementation((url) => {
    if (url === '/api/cyber-guard/overview') {
      return Promise.resolve({ data: { success: true, data: overviewData } })
    }
    if (url === '/api/cyber-guard/events' || url === '/api/cyber-guard/bans') {
      return Promise.resolve({ data: { success: true, data: emptyPage } })
    }
    if (url === '/api/cyber-guard/settings') {
      return Promise.resolve({
        data: { success: true, data: { ban_group: 'banned' } },
      })
    }
    return Promise.reject(new Error(`unexpected GET ${url}`))
  })
  const root = createRootRoute()
  const authenticated = createRoute({
    getParentRoute: () => root,
    id: '_authenticated',
  })
  const page = createRoute({
    getParentRoute: () => authenticated,
    path: 'admin-cyberguard/',
    validateSearch: cyberGuardSearchSchema,
    component: CyberGuardAdminPage,
  })
  const router = createRouter({
    routeTree: root.addChildren([authenticated.addChildren([page])]),
    history: createMemoryHistory({ initialEntries: [initialUrl] }),
  })
  await router.load()
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
      <Toaster />
    </QueryClientProvider>
  )
  return { get, router }
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('CyberGuard page', () => {
  test('without a tab the overview shows the mode and disables polling on a slave node', async () => {
    await renderPage('/admin-cyberguard/')

    expect(
      await screen.findByRole('tab', { name: 'Overview', selected: true })
    ).toBeVisible()
    expect(await screen.findByText('Mode: Observe only')).toBeVisible()
    expect(screen.getByRole('button', { name: 'Poll now' })).toBeDisabled()
    expect(
      screen.getByText(
        'This server is not the master node. Polls run on the master node only.'
      )
    ).toBeVisible()
  })

  test('a userId in the URL narrows the events request and can be cleared', async () => {
    const user = userEvent.setup()
    const { get, router } = await renderPage(
      '/admin-cyberguard/?tab=events&userId=2752'
    )

    expect(
      await screen.findByRole('tab', { name: 'Hit details', selected: true })
    ).toBeVisible()
    await waitFor(() =>
      expect(get).toHaveBeenCalledWith('/api/cyber-guard/events', {
        params: expect.objectContaining({ user_id: 2752, p: 1 }),
      })
    )
    await user.click(screen.getByRole('button', { name: 'Clear user filter' }))

    await waitFor(() =>
      expect(router.state.location.search).toEqual({ tab: 'events' })
    )
    expect(screen.queryByText('Only UID 2752')).toBeNull()
  })

  test('choosing a tab writes it to the URL', async () => {
    const user = userEvent.setup()
    const { router } = await renderPage('/admin-cyberguard/')

    await user.click(await screen.findByRole('tab', { name: 'Ban records' }))

    await waitFor(() =>
      expect(router.state.location.search).toEqual({ tab: 'bans' })
    )
    expect(
      await screen.findByRole('button', { name: 'Import existing bans' })
    ).toBeVisible()
  })

  test('a partial catch-up poll shows Catching up with a hint instead of a failure', async () => {
    await renderPage('/admin-cyberguard/', {
      ...overview,
      last_poll: {
        ...overview.last_poll,
        at: 1_760_000_000,
        ok: true,
        status: 'partial',
        fetched: 20_000,
        new_events: 12,
        cursor_end: 1_759_990_000,
      },
    })

    expect(await screen.findByText('Catching up')).toBeVisible()
    expect(
      screen.getByText(
        'Catching up on a backlog. The next poll continues where this one stopped.'
      )
    ).toBeVisible()
    expect(screen.queryByText('Failed')).toBeNull()
  })

  test('a manual poll that only caught up partly reports progress, not a failure', async () => {
    const user = userEvent.setup()
    vi.spyOn(api, 'post').mockResolvedValue({
      data: {
        success: true,
        data: {
          status: 'partial',
          window_start: 1_759_990_000,
          window_end: 1_760_000_000,
          fetched: 20_000,
          new_events: 12,
          attributed: 12,
          unattributed: 0,
          counted: 3,
          bans_applied: 1,
          proposals: 0,
          escalated: 0,
          expired: 0,
          drift_reapplied: 0,
        },
      },
    })
    await renderPage('/admin-cyberguard/', {
      ...overview,
      is_master_node: true,
    })

    await user.click(await screen.findByRole('button', { name: 'Poll now' }))

    expect(
      await screen.findByText(
        'Poll caught up partly: 20000 rows read, 12 new events, 1 bans applied. The next poll continues with the rest of the backlog.'
      )
    ).toBeVisible()
    expect(screen.queryByText(/^Poll failed/)).toBeNull()
  })

  test('an unknown tab in the URL falls back to the overview', async () => {
    await renderPage('/admin-cyberguard/?tab=nope&userId=-3')

    expect(
      await screen.findByRole('tab', { name: 'Overview', selected: true })
    ).toBeVisible()
  })
})

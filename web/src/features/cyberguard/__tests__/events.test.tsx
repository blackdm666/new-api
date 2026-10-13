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
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { CyberGuardAdminPage } from '..'
import { cyberGuardSearchSchema } from '../lib/search'
import type { CyberGuardEvent } from '../types'

const OCCURRED_AT = 1_760_000_000

function event(id: number, overrides: Partial<CyberGuardEvent> = {}) {
  const base: CyberGuardEvent = {
    id,
    ops_id: 9000 + id,
    occurred_at: OCCURRED_AT,
    sub2api_request_id: `req-${id}`,
    api_key_id: 1,
    account_id: 31,
    account_name: 'openai-31',
    category: 'cyber',
    error_type: 'cyber_policy',
    status_code: 400,
    endpoint: '/v1/responses',
    model: 'gpt-5.5',
    message: 'This request has been flagged for possible cybersecurity risk.',
    user_id: 2752,
    username: 'reseller2752',
    token_id: 4,
    token_name: 'codex',
    token_group: '代理商',
    channel_id: 9,
    log_id: 77,
    session_fp: 'a1b2c3d4',
    unit_key: 'fp:a1b2c3d4',
    is_new_unit: true,
    state: 'counted',
    void_reason: '',
    voided_by: 0,
    voided_at: 0,
    created_at: OCCURRED_AT,
  }
  return { ...base, ...overrides }
}

async function renderEvents(rows: CyberGuardEvent[]) {
  vi.spyOn(api, 'get').mockImplementation((url) => {
    if (url === '/api/cyber-guard/events') {
      return Promise.resolve({
        data: {
          success: true,
          data: { page: 1, page_size: 20, total: rows.length, items: rows },
        },
      })
    }
    if (url === '/api/cyber-guard/overview') {
      return Promise.reject(new Error('overview not needed'))
    }
    return Promise.reject(new Error(`unexpected GET ${url}`))
  })
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: true, data: { ...rows[0], state: 'void' } },
  })
  const root = createRootRoute()
  const authenticated = createRoute({
    getParentRoute: () => root,
    id: '_authenticated',
  })
  const routes = [
    createRoute({
      getParentRoute: () => authenticated,
      path: 'admin-cyberguard/',
      validateSearch: cyberGuardSearchSchema,
      component: CyberGuardAdminPage,
    }),
    createRoute({
      getParentRoute: () => authenticated,
      path: 'usage-logs/$section',
      component: () => <p>Usage logs</p>,
    }),
  ]
  const router = createRouter({
    routeTree: root.addChildren([authenticated.addChildren(routes)]),
    history: createMemoryHistory({
      initialEntries: ['/admin-cyberguard/?tab=events'],
    }),
  })
  await router.load()
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return post
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('hit details', () => {
  test('the sub2api request ID links to the usage log around the event time', async () => {
    await renderEvents([event(11)])

    const link = await screen.findByRole('link', { name: 'req-11' })
    const href = new URL(link.getAttribute('href') ?? '', 'http://localhost')

    expect(href.pathname).toBe('/usage-logs/common')
    expect(href.searchParams.get('upstreamRequestId')).toBe('req-11')
    expect(Number(href.searchParams.get('startTime'))).toBe(
      (OCCURRED_AT - 3600) * 1000
    )
    expect(Number(href.searchParams.get('endTime'))).toBe(
      (OCCURRED_AT + 3600) * 1000
    )
  })

  test('voiding requires a reason and posts it for that event', async () => {
    const user = userEvent.setup()
    const post = await renderEvents([event(11)])

    await user.click(await screen.findByRole('button', { name: 'Void' }))
    const dialog = await screen.findByRole('alertdialog')
    const confirm = within(dialog).getByRole('button', { name: 'Void' })
    expect(confirm).toBeDisabled()
    await user.type(within(dialog).getByLabelText('Reason'), 'false positive')
    await user.click(confirm)

    await waitFor(() =>
      expect(post).toHaveBeenCalledWith('/api/cyber-guard/events/11/void', {
        reason: 'false positive',
      })
    )
  })

  test('voided and unattributed rows show their state without a void action', async () => {
    await renderEvents([
      event(12, { state: 'void', void_reason: 'test traffic' }),
      event(13, {
        state: 'unattributed',
        user_id: 0,
        username: '',
        sub2api_request_id: '',
      }),
    ])

    const voided = (
      await screen.findByRole('link', { name: 'req-12' })
    ).closest('tr')
    const unattributed = screen.getByText('Not attributed').closest('tr')
    expect(voided).not.toBeNull()
    expect(unattributed).not.toBeNull()
    expect(within(voided as HTMLElement).getByText('Voided')).toBeVisible()
    expect(
      within(voided as HTMLElement).queryByRole('button', { name: 'Void' })
    ).toBeNull()
    expect(within(unattributed as HTMLElement).queryByRole('link')).toBeNull()
  })
})

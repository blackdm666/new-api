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
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { buildSearchParams } from '../filter'
import { fetchLogsByCategory } from '../utils'

afterEach(() => {
  vi.restoreAllMocks()
})

function mockTaskList() {
  return vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { items: [], total: 0 } },
  })
}

function requestedParams(
  spy: ReturnType<typeof mockTaskList>
): Record<string, string> {
  const url = String(spy.mock.calls[0]?.[0] ?? '')
  return Object.fromEntries(new URLSearchParams(url.split('?')[1] ?? ''))
}

describe('task log filters', () => {
  test('keeps the task ID, model and username in the page search params', () => {
    expect(
      buildSearchParams(
        { taskId: '385412976501234', model: 'veo%', username: 'alice' },
        'task'
      )
    ).toEqual({ filter: '385412976501234', model: 'veo%', username: 'alice' })
  })

  test('sends the task ID, model and username to the admin task list', async () => {
    const spy = mockTaskList()
    await fetchLogsByCategory({
      logCategory: 'task',
      isAdmin: true,
      page: 1,
      pageSize: 10,
      searchParams: { filter: 'cgt-1', model: 'veo-3.1', username: 'alice' },
      columnFilters: [],
    })
    expect(String(spy.mock.calls[0]?.[0])).toContain('/api/task')
    expect(requestedParams(spy)).toMatchObject({
      task_id: 'cgt-1',
      model_name: 'veo-3.1',
      username: 'alice',
    })
  })

  test('never sends a username from the user task list', async () => {
    const spy = mockTaskList()
    await fetchLogsByCategory({
      logCategory: 'task',
      isAdmin: false,
      page: 1,
      pageSize: 10,
      searchParams: { model: 'veo-3.1', username: 'alice' },
      columnFilters: [],
    })
    const params = requestedParams(spy)
    expect(params.model_name).toBe('veo-3.1')
    expect(params).not.toHaveProperty('username')
  })
})

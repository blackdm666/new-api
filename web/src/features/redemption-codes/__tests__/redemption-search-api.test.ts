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
import type { AxiosAdapter, InternalAxiosRequestConfig } from 'axios'
import { afterEach, describe, expect, test } from 'vitest'

import { api } from '@/lib/api'

import { searchRedemptions } from '../api'

describe('redemption code search API', () => {
  const originalAdapter = api.defaults.adapter

  afterEach(() => {
    api.defaults.adapter = originalAdapter
  })

  test('sends name, code, and ID as independent query conditions', async () => {
    let captured: InternalAxiosRequestConfig | undefined
    const adapter: AxiosAdapter = async (config) => {
      captured = config
      return {
        config,
        data: {
          success: true,
          data: { items: [], total: 0, page: 2, page_size: 20 },
        },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }
    api.defaults.adapter = adapter

    await searchRedemptions({
      name: 'campaign-a',
      code: '10000000000000000000000000000012',
      id: '12',
      status: '1',
      p: 2,
      page_size: 20,
    })

    expect(captured?.method).toBe('get')
    expect(captured?.url).toBe(
      '/api/redemption/search?name=campaign-a&code=10000000000000000000000000000012&id=12&status=1&p=2&page_size=20'
    )
  })
})

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
import assert from 'node:assert/strict'

import type { AxiosAdapter, InternalAxiosRequestConfig } from 'axios'
import { afterAll, describe, test } from 'vitest'

import { api } from '@/lib/api'

import { fetchAdminAffiliateInviteRecords } from '../api'

describe('admin affiliate invitation records API', () => {
  const originalAdapter = api.defaults.adapter

  afterAll(() => {
    api.defaults.adapter = originalAdapter
  })

  test('requests the global invitation endpoint with search and pagination', async () => {
    let captured: InternalAxiosRequestConfig | undefined
    const adapter: AxiosAdapter = async (config) => {
      captured = config
      return {
        config,
        data: {
          success: true,
          data: {
            items: [
              {
                inviter_id: 42,
                inviter_username: 'promoter',
                inviter_display_name: 'Promoter',
                invitee_id: 84,
                invitee_username: 'invitee',
                invitee_display_name: 'Invitee',
                created_at: 1_786_700_000,
                is_new: false,
                topup_count: 0,
                topup_amount_cents: 0,
                commission_cents: 0,
                last_topup_time: 0,
              },
            ],
            total: 1,
          },
        },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }
    api.defaults.adapter = adapter

    const result = await fetchAdminAffiliateInviteRecords({
      page: 2,
      pageSize: 10,
      keyword: 'promoter 42',
    })

    assert.equal(
      captured?.url,
      '/api/affiliate/admin/invitees?p=2&page_size=10&keyword=promoter+42'
    )
    assert.equal(captured?.method, 'get')
    assert.equal(result.total, 1)
    assert.equal(result.items[0]?.topup_count, 0)
  })
})

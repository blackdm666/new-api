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
import { after, describe, test } from 'node:test'

import type { AxiosAdapter, InternalAxiosRequestConfig } from 'axios'

import { api } from '@/lib/api'

import { approveAffiliateUpgrade, updateAffiliateSettings } from '../api'

describe('affiliate upgrade approval API', () => {
  const originalAdapter = api.defaults.adapter

  after(() => {
    api.defaults.adapter = originalAdapter
  })

  test('sends the reviewed target group so a repeated request cannot skip a tier', async () => {
    let captured: InternalAxiosRequestConfig | undefined
    const adapter: AxiosAdapter = async (config) => {
      captured = config
      return {
        config,
        data: { success: true, data: {} },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }
    api.defaults.adapter = adapter

    await approveAffiliateUpgrade(42, '金牌推广')

    assert.equal(
      captured?.url,
      '/api/affiliate/admin/upgrade-candidates/42/approve'
    )
    assert.equal(captured?.method, 'post')
    assert.deepEqual(JSON.parse(String(captured?.data)), {
      next_group: '金牌推广',
    })
  })

  test('saves both people and top-up amount thresholds atomically', async () => {
    let captured: InternalAxiosRequestConfig | undefined
    const adapter: AxiosAdapter = async (config) => {
      captured = config
      return {
        config,
        data: { success: true, data: {} },
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }
    api.defaults.adapter = adapter

    await updateAffiliateSettings({
      enabled: true,
      auto_approve: false,
      default_rate_basis_points: 500,
      group_rates: { default: 500, 高级推广: 1000, 金牌推广: 1500 },
      upgrade_invitees_threshold: 50,
      gold_upgrade_invitees_threshold: 500,
      upgrade_top_up_amount_threshold_cents: 200000,
      gold_upgrade_top_up_amount_threshold_cents: 2000000,
    })

    assert.equal(captured?.url, '/api/affiliate/root/settings')
    assert.equal(captured?.method, 'put')
    const payload = JSON.parse(String(captured?.data))
    assert.equal(payload.upgrade_invitees_threshold, 50)
    assert.equal(payload.gold_upgrade_invitees_threshold, 500)
    assert.equal(payload.upgrade_top_up_amount_threshold_cents, 200000)
    assert.equal(payload.gold_upgrade_top_up_amount_threshold_cents, 2000000)
  })
})

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
import type { TFunction } from 'i18next'
import { beforeEach, describe, expect, test } from 'vitest'

import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { usageLogSchema } from '../../data/schema'
import { buildBillingBreakdownRows } from '../../lib/billing-breakdown'
import { buildTypeDetailSegments } from '../../lib/log-detail-segments'
import type { LogOtherData } from '../../types'

const t = ((key: string, options?: Record<string, unknown>) =>
  key.replace('{{ratio}}', String(options?.ratio ?? ''))) as TFunction

const log = usageLogSchema.parse({
  id: 1,
  user_id: 1,
  created_at: 1,
  type: 2,
  content: '',
})

describe('usage log actual price display', () => {
  beforeEach(() => {
    useSystemConfigStore.getState().setConfig({
      currency: {
        ...DEFAULT_CURRENCY_CONFIG,
        quotaDisplayType: 'CNY',
        usdExchangeRate: 7,
      },
    })
  })

  test('shows group-adjusted token and cache prices in the billing card', () => {
    const other: LogOtherData = {
      model_ratio: 2.5,
      completion_ratio: 5,
      group_ratio: 0.3,
      claude: true,
      cache_tokens: 1,
      cache_creation_tokens_5m: 1,
      cache_ratio: 0.1,
      cache_creation_ratio: 1.25,
      cache_creation_ratio_5m: 1.25,
    }

    const rows = buildBillingBreakdownRows(log, other, false, t)

    expect(rows.slice(0, 7)).toEqual([
      { label: 'Billing Mode', value: 'Per-token' },
      {
        label: 'Price Explanation',
        value: 'Adjusted by 0.3x group rate',
      },
      { label: 'Input', value: '¥10.5/M' },
      { label: 'Output', value: '¥52.5/M' },
      { label: 'Cache Read', value: '¥1.05/M' },
      { label: 'Cache Creation', value: '¥13.125/M' },
      { label: 'Cache Creation (5m)', value: '¥13.125/M' },
    ])
  })

  test('shows the same adjusted prices in the log table detail column', () => {
    const other: LogOtherData = {
      model_ratio: 2.5,
      completion_ratio: 5,
      group_ratio: 0.3,
      claude: true,
      cache_tokens: 1,
      cache_ratio: 0.1,
      cache_creation_ratio: 1.25,
    }

    expect(buildTypeDetailSegments(log, other, t)).toEqual([
      { text: 'Standard · ¥10.5 / ¥52.5/M' },
      { text: 'Cache ¥1.05 / ¥13.125', muted: true },
    ])
  })

  test('adjusts per-call prices in both locations', () => {
    const other: LogOtherData = {
      model_price: 0.5,
      group_ratio: 0.3,
    }

    expect(buildTypeDetailSegments(log, other, t)[0]).toEqual({
      text: 'Per-call · ¥1.05',
    })
    expect(buildBillingBreakdownRows(log, other, false, t).slice(0, 3)).toEqual(
      [
        { label: 'Billing Mode', value: 'Per-call' },
        {
          label: 'Price Explanation',
          value: 'Adjusted by 0.3x group rate',
        },
        { label: 'Model Price', value: '¥1.05' },
      ]
    )
  })
})

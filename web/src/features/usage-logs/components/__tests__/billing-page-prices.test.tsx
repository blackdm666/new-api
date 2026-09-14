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
import { flexRender } from '@tanstack/react-table'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, it } from 'vitest'

import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { usageLogSchema, type UsageLog } from '../../data/schema'
import type { LogOtherData } from '../../types'
import { useCommonLogsColumns } from '../columns/common-logs-columns'
import { DetailsDialog } from '../dialogs/details-dialog'

let client: QueryClient
const originalConfig = useSystemConfigStore.getState().config
beforeEach(() => {
  client = new QueryClient({
    defaultOptions: { queries: { enabled: false, retry: false } },
  })
  useSystemConfigStore.getState().setConfig({
    currency: { ...DEFAULT_CURRENCY_CONFIG, quotaDisplayType: 'USD' },
  })
})
afterEach(() => {
  cleanup()
  client.clear()
  useSystemConfigStore.getState().setConfig(originalConfig)
})

function LogContentCell(props: { log: UsageLog }) {
  const column = useCommonLogsColumns(false, false).find(
    (entry) => 'accessorKey' in entry && entry.accessorKey === 'content'
  )
  if (!column) throw new Error('Missing content column')
  return (
    <>{flexRender(column.cell, { row: { original: props.log } } as never)}</>
  )
}

it.each([
  {
    label: 'discounted token',
    other: { model_ratio: 1, completion_ratio: 2, group_ratio: 0.5 },
    text: 'Standard · $1 / $2/M',
    detail: '$1/M',
  },
  {
    label: 'per-second video',
    other: {
      model_price: 0.33,
      billing_unit: 'second',
      is_task: true,
      task_ratios: { seconds: 4 },
      group_ratio: 1,
    },
    text: 'Per-second · $0.33/second',
    detail: '$0.33/second',
  },
  {
    label: 'zero-rate token',
    other: { model_ratio: 1, completion_ratio: 2, group_ratio: 0 },
    text: 'Standard · $0 / $0/M',
    detail: '$0/M',
  },
  {
    label: 'user-exclusive token',
    other: {
      model_ratio: 1,
      completion_ratio: 2,
      group_ratio: 0.5,
      user_group_ratio: 0.2,
    },
    text: 'Standard · $0.4 / $0.8/M',
    detail: '$0.4/M',
  },
  {
    label: 'dynamic per-request',
    other: {
      billing_mode: 'tiered_expr',
      expr_b64: btoa('tier("Standard", fixed(0.4))'),
      matched_tier: 'Standard',
      group_ratio: 0.5,
    },
    text: 'Standard · Per-call $0.2/request',
    detail: '$0.2/request',
  },
])(
  'shows $label prices through the actual log column and details dialog',
  ({ other, text, detail }) => {
    const log = usageLogSchema.parse({
      id: 1,
      user_id: 1,
      created_at: 1,
      type: 2,
      content: '',
      other: JSON.stringify(other as LogOtherData),
    })
    const view = render(
      <QueryClientProvider client={client}>
        <LogContentCell log={log} />
      </QueryClientProvider>
    )
    expect(screen.getByText(text)).toBeVisible()
    view.rerender(
      <QueryClientProvider client={client}>
        <DetailsDialog log={log} isAdmin={false} open onOpenChange={() => {}} />
      </QueryClientProvider>
    )
    expect(screen.getAllByText(detail).length).toBeGreaterThan(0)
    if (other.billing_unit === 'second') {
      expect(screen.getByText('4s')).toBeVisible()
    }
  }
)

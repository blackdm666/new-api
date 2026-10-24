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
import { cleanup, render, screen, within } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import type { PerformanceGroup } from '@/features/performance-metrics/types'

import type { PricingModel } from '../../types'
import { ModelDetailsPerformance } from '../model-details-performance'

// Chart rendering needs browser canvas.
vi.mock('@visactor/react-vchart', () => ({ VChart: () => null }))

const client = new QueryClient({
  defaultOptions: { queries: { enabled: false } },
})

const model: PricingModel = {
  id: 1,
  model_name: 'gpt-6-sol',
  quota_type: 0,
  model_ratio: 1,
  completion_ratio: 1,
  enable_groups: ['GPT-Plus', 'AZ-GPT-T6号池'],
}

function perfGroup(group: string): PerformanceGroup {
  return {
    group,
    avg_ttft_ms: 1000,
    avg_latency_ms: 2000,
    success_rate: 100,
    avg_tps: 30,
    series: [],
  }
}

afterEach(() => {
  cleanup()
  client.clear()
})

it('lists groups pinned to the bottom after the other groups in the per-group performance table', () => {
  client.setQueryData(['pricing'], {
    success: true,
    data: [],
    vendors: [],
    group_ratio: {},
    usable_group: {},
    group_pin_bottom: ['AZ-GPT-T6号池'],
    supported_endpoint: {},
    auto_groups: [],
  })
  client.setQueryData(['perf-metrics', model.model_name], {
    success: true,
    data: {
      enabled: true,
      model_name: model.model_name,
      summary: { avg_latency_ms: 2000, success_rate: 100, avg_tps: 30 },
      series: [],
      groups: [
        perfGroup('AZ-GPT-T6号池'),
        perfGroup('GPT-Plus'),
        perfGroup('GPT-Pro'),
      ],
    },
  })

  render(
    <QueryClientProvider client={client}>
      <ModelDetailsPerformance model={model} />
    </QueryClientProvider>
  )

  const table = screen.getAllByRole('table')[0]
  const groupCells = within(table)
    .getAllByRole('row')
    .slice(1)
    .map((row) => within(row).getAllByRole('cell')[0].textContent)
  expect(groupCells).toEqual(['GPT-Plus', 'GPT-Pro', 'AZ-GPT-T6号池'])
})

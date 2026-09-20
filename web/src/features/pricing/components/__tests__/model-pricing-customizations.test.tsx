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
import i18next from 'i18next'
import { afterEach, expect, it, vi } from 'vitest'

import zh from '@/i18n/locales/zh.json'

import type { PricingModel } from '../../types'
import { ModelDetailsContent } from '../model-details'
import { ModelPriceCell } from '../model-price-cell'
import { usePricingColumns } from '../pricing-columns'

const client = new QueryClient({
  defaultOptions: { queries: { enabled: false } },
})
// Chart rendering needs browser canvas; pricing is rendered through the real page.
vi.mock('@visactor/react-vchart', () => ({ VChart: () => null }))
const model: PricingModel = {
  id: 1,
  model_name: 'reference-video',
  quota_type: 1,
  model_ratio: 0,
  completion_ratio: 0,
  model_price: 0.33,
  billing_unit: 'second',
  enable_groups: ['test'],
}

afterEach(async () => {
  cleanup()
  client.clear()
  await i18next.changeLanguage('en')
})

it('shows per-second units in the actual price table cell', () => {
  render(<ModelPriceCell model={model} />)
  expect(screen.getByText('Per-second')).toBeVisible()
  expect(screen.getByText('USD / second')).toBeVisible()
  expect(screen.queryByText('Per-request')).not.toBeInTheDocument()
})

it.each([
  ['text', false],
  ['dynamic', true],
] as const)(
  'renders Chinese discounts and group remarks in the %s model detail',
  async (_, dynamic) => {
    i18next.addResourceBundle('zh', 'translation', zh.translation, true, true)
    await i18next.changeLanguage('zh')
    render(
      <QueryClientProvider client={client}>
        <ModelDetailsContent
          model={{
            ...model,
            quota_type: 0,
            model_ratio: 1,
            completion_ratio: 2,
            billing_unit: undefined,
            ...(dynamic
              ? {
                  billing_mode: 'tiered_expr',
                  billing_expr: 'tier("default", p * 1 + c * 2)',
                }
              : {}),
          }}
          groupRatio={{ test: 0.5 }}
          usableGroup={{ test: { desc: '回归测试分组说明', ratio: 0.5 } }}
          endpointMap={{}}
          autoGroups={[]}
          priceRate={1}
          usdExchangeRate={1}
          tokenUnit='M'
        />
      </QueryClientProvider>
    )
    expect(screen.getByText('回归测试分组说明')).toBeVisible()
    expect(screen.getByText('5折')).toBeVisible()
  }
)

it('preserves a zero group multiplier in the model detail', () => {
  render(
    <QueryClientProvider client={client}>
      <ModelDetailsContent
        model={model}
        groupRatio={{ test: 0 }}
        usableGroup={{ test: 'Free test group' }}
        endpointMap={{}}
        autoGroups={[]}
        priceRate={1}
        usdExchangeRate={1}
        tokenUnit='M'
      />
    </QueryClientProvider>
  )
  expect(screen.getByText('0x')).toBeVisible()
})

function PricingNameCell(props: { model: PricingModel }) {
  const column = usePricingColumns().find(
    (entry) => 'accessorKey' in entry && entry.accessorKey === 'model_name'
  )
  if (!column) throw new Error('Missing model name column')
  return (
    <>{flexRender(column.cell, { row: { original: props.model } } as never)}</>
  )
}

it('shows the Hunyuan fallback icon in the pricing table without overriding configured icons', async () => {
  const view = render(
    <PricingNameCell model={{ ...model, model_name: 'hunyuan-video' }} />
  )
  expect(await screen.findByTitle('Hunyuan')).toBeInTheDocument()
  view.rerender(
    <PricingNameCell
      model={{ ...model, model_name: 'hunyuan-video', icon: 'OpenAI' }}
    />
  )
  expect(screen.queryByTitle('Hunyuan')).not.toBeInTheDocument()
  expect(await screen.findByTitle('OpenAI')).toBeInTheDocument()
})

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
import { describe, expect, test } from 'vitest'

import { SORT_OPTIONS } from '../constants'
import { filterAndSortModels, sortModels } from '../lib/filters'
import type { PricingModel } from '../types'

function pricingModel(
  modelName: string,
  overrides: Partial<PricingModel>
): PricingModel {
  return {
    id: 1,
    model_name: modelName,
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 1,
    enable_groups: ['default'],
    ...overrides,
  }
}

const models = [
  pricingModel('legacy-ratio', { model_ratio: 1, completion_ratio: 4 }),
  pricingModel('legacy-per-call', { quota_type: 1, model_price: 0.05 }),
  pricingModel('expr-tokens', {
    billing_mode: 'tiered_expr',
    billing_expr: 'tier("base", p * 1 + c * 10)',
  }),
  pricingModel('expr-long-context', {
    billing_mode: 'tiered_expr',
    billing_expr:
      'len <= 200000 ? tier("standard", p * 3 + c * 15) : tier("long", p * 6 + c * 22.5)',
  }),
  pricingModel('expr-input-only', {
    billing_mode: 'tiered_expr',
    billing_expr: 'tier("base", p * 0.1)',
  }),
  pricingModel('expr-fixed', {
    billing_mode: 'tiered_expr',
    billing_expr: 'tier("request", fixed(0.02))',
  }),
  pricingModel('expr-task', {
    billing_mode: 'tiered_expr',
    billing_expr: 'tier("base", u("seconds") * 0.1)',
    billing_usage_schema: { seconds: { type: 'number', unit: 'second' } },
    billing_usage_examples: [{ label: '5s', facts: { seconds: 5 } }],
  }),
]

describe('price sorting', () => {
  test.each([
    [
      SORT_OPTIONS.INPUT_PRICE_LOW,
      [
        'expr-input-only',
        'expr-tokens',
        'legacy-ratio',
        'expr-long-context',
        'legacy-per-call',
        'expr-fixed',
        'expr-task',
      ],
    ],
    [
      SORT_OPTIONS.INPUT_PRICE_HIGH,
      [
        'expr-long-context',
        'legacy-ratio',
        'expr-tokens',
        'expr-input-only',
        'legacy-per-call',
        'expr-fixed',
        'expr-task',
      ],
    ],
    [
      SORT_OPTIONS.OUTPUT_PRICE_LOW,
      [
        'legacy-ratio',
        'expr-tokens',
        'expr-long-context',
        'legacy-per-call',
        'expr-input-only',
        'expr-fixed',
        'expr-task',
      ],
    ],
    [
      SORT_OPTIONS.REQUEST_PRICE_HIGH,
      [
        'expr-task',
        'legacy-per-call',
        'expr-fixed',
        'legacy-ratio',
        'expr-tokens',
        'expr-long-context',
        'expr-input-only',
      ],
    ],
  ])(
    '%s compares base prices across legacy and expression billing and keeps unpriced models last',
    (sortBy, expected) => {
      expect(sortModels(models, sortBy).map((m) => m.model_name)).toEqual(
        expected
      )
    }
  )
})

describe('pinned group sorting', () => {
  const pinnedGroups = ['AZ-GPT-T6号池', 'AZ-GPT可蒸馏']
  const catalog = [
    pricingModel('az-gpt-image-2', { enable_groups: ['AZ-GPT-T6号池'] }),
    pricingModel('claude-fable-5', { enable_groups: ['Claude Kiro'] }),
    pricingModel('gpt-5.5', { enable_groups: ['AZ-GPT-T6号池', 'auto'] }),
    pricingModel('gpt-6-sol', {
      enable_groups: ['AZ-GPT-T6号池', 'GPT-Plus'],
    }),
  ]
  const filters = {
    search: '',
    vendor: 'all',
    group: 'all',
    quotaType: 'all',
    endpointType: 'all',
    tag: 'all',
    sortBy: SORT_OPTIONS.NAME,
  }

  test('models offered only in pinned groups move after the rest in name order', () => {
    expect(
      filterAndSortModels(catalog, filters, pinnedGroups).map(
        (m) => m.model_name
      )
    ).toEqual(['claude-fable-5', 'gpt-6-sol', 'az-gpt-image-2', 'gpt-5.5'])
  })

  test('without pinned groups the name order is unchanged', () => {
    expect(
      filterAndSortModels(catalog, filters).map((m) => m.model_name)
    ).toEqual(['az-gpt-image-2', 'claude-fable-5', 'gpt-5.5', 'gpt-6-sol'])
  })
})

describe('pinned model sorting', () => {
  const catalog = [
    pricingModel('az-gpt-image-2', { enable_groups: ['AZ-GPT-T6号池'] }),
    pricingModel('az-gpt-image-3', { enable_groups: ['AZ-GPT-T6号池'] }),
    pricingModel('claude-fable-5', { enable_groups: ['Claude Kiro'] }),
    pricingModel('gpt-5.5', { enable_groups: ['GPT-Plus'] }),
    pricingModel('gpt-6-sol', { enable_groups: ['GPT-Plus'] }),
  ]
  const filters = {
    search: '',
    vendor: 'all',
    group: 'all',
    quotaType: 'all',
    endpointType: 'all',
    tag: 'all',
    sortBy: SORT_OPTIONS.NAME,
  }
  const pins = {
    top: ['gpt-6-sol', 'az-gpt-image-3'],
    bottom: ['claude-fable-5'],
  }

  test('all groups keep the name order whatever the pins', () => {
    expect(
      filterAndSortModels(catalog, filters, [], pins).map((m) => m.model_name)
    ).toEqual([
      'az-gpt-image-2',
      'az-gpt-image-3',
      'claude-fable-5',
      'gpt-5.5',
      'gpt-6-sol',
    ])
  })

  test('a selected group lists its pinned models first and last', () => {
    expect(
      filterAndSortModels(catalog, { ...filters, group: 'GPT-Plus' }, [], {
        top: ['gpt-6-sol'],
        bottom: ['gpt-5.5', 'claude-fable-5'],
      }).map((m) => m.model_name)
    ).toEqual(['gpt-6-sol', 'gpt-5.5'])
  })

  test('in a selected group pinned models stay inside the pinned-group part they belong to', () => {
    const shared = [
      ...catalog,
      pricingModel('gpt-7', { enable_groups: ['AZ-GPT-T6号池', 'GPT-Plus'] }),
    ]
    expect(
      filterAndSortModels(
        shared,
        { ...filters, group: 'AZ-GPT-T6号池' },
        ['AZ-GPT-T6号池'],
        { top: ['az-gpt-image-3'], bottom: [] }
      ).map((m) => m.model_name)
    ).toEqual(['gpt-7', 'az-gpt-image-3', 'az-gpt-image-2'])
  })

  test('an explicit price sort ignores model pins', () => {
    const priced = [
      pricingModel('cheap', { model_ratio: 0.5 }),
      pricingModel('pricey', { model_ratio: 5 }),
    ]
    expect(
      filterAndSortModels(
        priced,
        { ...filters, group: 'default', sortBy: SORT_OPTIONS.INPUT_PRICE_LOW },
        [],
        { top: ['pricey'], bottom: [] }
      ).map((m) => m.model_name)
    ).toEqual(['cheap', 'pricey'])
  })
})

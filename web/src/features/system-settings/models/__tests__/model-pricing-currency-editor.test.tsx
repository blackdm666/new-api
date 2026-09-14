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
import { act, fireEvent, render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { createRef } from 'react'
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  test,
  vi,
} from 'vitest'

import { usePricingPreferencesStore } from '@/stores/pricing-preferences-store'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import {
  ModelPricingEditorPanel,
  type ModelPricingEditorPanelHandle,
} from '../model-pricing-sheet'

vi.mock('@/features/pricing/hooks/use-pricing-data', () => ({
  usePricingData: () => ({ models: [] }),
}))

describe('model pricing editor currency', () => {
  beforeEach(() => {
    usePricingPreferencesStore.setState(
      usePricingPreferencesStore.getInitialState()
    )
    localStorage.clear()
  })

  beforeAll(() => {
    i18next.addResourceBundle('en', 'translation', {
      'Per-second': 'Per-second',
      'Price per second': 'Price per second',
      'per second': 'per second',
      'Price in {{currency}} per generated second. Requires a task adapter that reports duration.':
        'Price in {{currency}} per generated second. Requires a task adapter that reports duration.',
      CNY: 'CNY',
    })
  })

  afterEach(() => {
    usePricingPreferencesStore.setState(
      usePricingPreferencesStore.getInitialState()
    )
    localStorage.clear()
    useSystemConfigStore.getState().setConfig({
      currency: { ...DEFAULT_CURRENCY_CONFIG },
    })
  })

  test('defaults to site CNY without a saved preference and commits the internal USD ModelPrice', async () => {
    useSystemConfigStore.getState().setConfig({
      currency: {
        ...DEFAULT_CURRENCY_CONFIG,
        quotaDisplayType: 'CNY',
        usdExchangeRate: 7,
      },
    })
    const ref = createRef<ModelPricingEditorPanelHandle>()

    render(
      <QueryClientProvider client={new QueryClient()}>
        <ModelPricingEditorPanel
          ref={ref}
          editData={{
            name: 'video-model',
            price: '0.2',
            billingMode: 'per-second',
          }}
        />
      </QueryClientProvider>
    )

    expect(
      screen.getByRole('combobox', { name: 'Pricing currency' })
    ).toHaveTextContent('Site currency (CNY)')
    const input = screen.getByDisplayValue('1.4')
    expect(screen.getAllByText('¥').length).toBeGreaterThan(0)
    expect(
      screen.getByText(
        'Price in CNY per generated second. Requires a task adapter that reports duration.'
      )
    ).toBeInTheDocument()

    fireEvent.change(input, { target: { value: '2.8' } })

    let result = null
    await act(async () => {
      result = (await ref.current?.commitDraft()) ?? null
    })

    expect(result).toMatchObject({
      name: 'video-model',
      price: '0.4',
      billingMode: 'per-second',
    })
  })
})

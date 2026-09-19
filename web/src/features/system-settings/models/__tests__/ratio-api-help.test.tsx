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
import { cleanup, render, screen } from '@testing-library/react'
import { useForm } from 'react-hook-form'
import { afterEach, expect, it } from 'vitest'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { ModelRatioForm } from '../model-ratio-form'

const client = new QueryClient({
  defaultOptions: { queries: { enabled: false } },
})
afterEach(() => {
  cleanup()
  client.clear()
})
const values = {
  ModelPrice: '{}',
  ModelRatio: '{}',
  CacheRatio: '{}',
  CreateCacheRatio: '{}',
  CompletionRatio: '{}',
  ImageRatio: '{}',
  AudioRatio: '{}',
  AudioCompletionRatio: '{}',
  ExposeRatioEnabled: true,
  BillingMode: '{}',
  BillingExpr: '{}',
  PluginBillingExpr: '{}',
}
function FormHarness() {
  const form = useForm({ defaultValues: values })
  return (
    <ModelRatioForm
      form={form}
      savedValues={values}
      onSave={async () => {}}
      onReset={() => {}}
      isSaving={false}
      isResetting={false}
    />
  )
}
it('documents the real ratio API route in the pricing settings control', () => {
  const actions = document.createElement('div')
  document.body.append(actions)
  try {
    render(
      <QueryClientProvider client={client}>
        <SettingsPageProvider actionsContainer={actions}>
          <FormHarness />
        </SettingsPageProvider>
      </QueryClientProvider>
    )
    expect(
      screen.getByRole('switch', { name: 'Expose ratio API' })
    ).toHaveAccessibleDescription(
      'Allow clients to query configured prices via `/api/ratio_config`.'
    )
  } finally {
    actions.remove()
  }
})

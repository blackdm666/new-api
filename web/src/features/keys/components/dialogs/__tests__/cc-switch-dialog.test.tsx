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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { CCSwitchDialog } from '../cc-switch-dialog'

let queryClient: QueryClient

beforeEach(() => {
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  // The console's own address differs from the API host used by the
  // CC Switch preset; the import link must not be derived from it.
  localStorage.setItem(
    'status',
    JSON.stringify({ server_address: 'https://88api.ai' })
  )
  vi.spyOn(window, 'open').mockReturnValue(null)
})

afterEach(() => {
  queryClient.clear()
  localStorage.clear()
  vi.restoreAllMocks()
})

function renderDialog(tokenKey = 'test-only') {
  const onOpenChange = vi.fn()
  render(
    <QueryClientProvider client={queryClient}>
      <CCSwitchDialog open onOpenChange={onOpenChange} tokenKey={tokenKey} />
    </QueryClientProvider>
  )
  return { onOpenChange }
}

function openedImportParams() {
  expect(window.open).toHaveBeenCalledTimes(1)
  const url = new URL(vi.mocked(window.open).mock.calls[0][0] as string)
  expect(`${url.protocol}//${url.host}${url.pathname}`).toBe(
    'ccswitch://v1/import'
  )
  return Object.fromEntries(url.searchParams)
}

describe('CC Switch 88API preset import', () => {
  it('imports Claude with the 88API preset fields and the selected key when opened with defaults', async () => {
    const { onOpenChange } = renderDialog()
    const user = userEvent.setup()

    await user.click(screen.getByRole('button', { name: 'Open CC Switch' }))

    expect(openedImportParams()).toEqual({
      resource: 'provider',
      app: 'claude',
      name: '88API',
      icon: '88api',
      homepage: 'https://88api.ai',
      endpoint: 'https://api.88api.ai,https://88api.ai',
      apiKey: 'sk-test-only',
      enabled: 'true',
    })
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it.each([
    {
      label: 'Codex',
      app: 'codex',
      endpoint: 'https://api.88api.ai/v1,https://88api.ai/v1',
      model: 'gpt-6.1-sol',
    },
    {
      label: 'Gemini CLI',
      app: 'gemini',
      endpoint: 'https://api.88api.ai,https://88api.ai',
      model: 'gemini-3.8-flash',
    },
    {
      label: 'Grok Build',
      app: 'grokbuild',
      endpoint: 'https://api.88api.ai/v1,https://88api.ai/v1',
      model: 'grok-4.5',
    },
  ])(
    'imports $label with the preset endpoint and model when $label is selected',
    async ({ label, app, endpoint, model }) => {
      renderDialog()
      const user = userEvent.setup()

      await user.click(screen.getByRole('radio', { name: label }))
      await user.click(screen.getByRole('button', { name: 'Open CC Switch' }))

      expect(openedImportParams()).toMatchObject({
        app,
        name: '88API',
        endpoint,
        model,
      })
    }
  )

  it('shows the preset name, API address and model for the selected app without editable fields', async () => {
    renderDialog()
    const user = userEvent.setup()

    expect(screen.getByText('88API')).toBeVisible()
    expect(screen.getByText('https://api.88api.ai')).toBeVisible()
    expect(screen.queryByText('Model')).not.toBeInTheDocument()
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
    expect(screen.queryByRole('combobox')).not.toBeInTheDocument()

    await user.click(screen.getByRole('radio', { name: 'Codex' }))

    expect(screen.getByText('https://api.88api.ai/v1')).toBeVisible()
    expect(screen.getByText('gpt-6.1-sol')).toBeVisible()
  })

  it('offers only the apps CC Switch can import by link and points the others to the in-app preset', () => {
    renderDialog()

    expect(screen.getAllByRole('radio')).toHaveLength(4)
    for (const name of ['Claude Code', 'Codex', 'Gemini CLI', 'Grok Build']) {
      expect(screen.getByRole('radio', { name })).toBeInTheDocument()
    }
    expect(screen.getByText(/Claude Desktop/)).toBeVisible()
  })

  it('keeps an existing sk- prefix when the key already has one', async () => {
    renderDialog('sk-test-only')
    const user = userEvent.setup()

    await user.click(screen.getByRole('button', { name: 'Open CC Switch' }))

    expect(openedImportParams().apiKey).toBe('sk-test-only')
  })
})

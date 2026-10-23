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

import { api } from '@/lib/api'

import type { ApiKey } from '../../../types'
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

function renderDialog({
  tokenKey = 'test-only',
  group = 'Claude Kiro',
  apiKey = null as ApiKey | null,
} = {}) {
  const onOpenChange = vi.fn()
  const onBalanceQueryEnabled = vi.fn()
  render(
    <QueryClientProvider client={queryClient}>
      <CCSwitchDialog
        open
        onOpenChange={onOpenChange}
        tokenKey={tokenKey}
        group={group}
        apiKey={apiKey}
        onBalanceQueryEnabled={onBalanceQueryEnabled}
      />
    </QueryClientProvider>
  )
  return { onOpenChange, onBalanceQueryEnabled }
}

function decodeUsageScript(encoded: string | undefined) {
  return new TextDecoder().decode(
    Uint8Array.from(atob(encoded ?? ''), (char) => char.charCodeAt(0))
  )
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

    const { usageScript, ...params } = openedImportParams()
    expect(params).toEqual({
      resource: 'provider',
      app: 'claude',
      name: '88API',
      icon: '88api',
      homepage: 'https://88api.ai',
      endpoint: 'https://api.88api.ai,https://88api.ai',
      apiKey: 'sk-test-only',
      enabled: 'true',
      usageEnabled: 'true',
      usageAutoInterval: '30',
    })
    expect(decodeUsageScript(usageScript)).toContain('/api/usage/token/')
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
      model: 'grok-4.7',
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

  it('shows the preset name, API address and default model for the selected app without editable fields', async () => {
    renderDialog()
    const user = userEvent.setup()

    expect(screen.getByText('88API')).toBeVisible()
    expect(screen.getByText('https://api.88api.ai')).toBeVisible()
    expect(screen.queryByText('Default model')).not.toBeInTheDocument()
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
    expect(screen.queryByRole('combobox')).not.toBeInTheDocument()

    await user.click(screen.getByRole('radio', { name: 'Codex' }))

    expect(screen.getByText('https://api.88api.ai/v1')).toBeVisible()
    expect(screen.getByText('Default model')).toBeVisible()
    expect(screen.getByText('gpt-6.1-sol')).toBeVisible()
    expect(
      screen.getByText(/only the initial choice.+any model in the key's group/)
    ).toBeVisible()
  })

  it('shows the key group and asks for a group with the models of the selected app', async () => {
    renderDialog({ group: 'Grok' })
    const user = userEvent.setup()

    await user.click(screen.getByRole('radio', { name: 'Grok Build' }))

    expect(screen.getByText('Grok', { exact: true })).toBeVisible()
    expect(screen.getByRole('alert')).toHaveTextContent(
      'Grok Build needs a key in a group that offers Grok models'
    )
  })

  it.each([
    { group: '', label: 'Follow user group' },
    { group: 'auto', label: 'Cross-group' },
  ])(
    'labels the key group as $label when the key group is "$group"',
    ({ group, label }) => {
      renderDialog({ group })

      expect(screen.getByText(label)).toBeVisible()
    }
  )

  it('offers Claude Code CLI and Claude Desktop separately and explains the remaining apps behind the info button', async () => {
    renderDialog()
    const user = userEvent.setup()

    expect(screen.getAllByRole('radio')).toHaveLength(5)
    for (const name of [
      'Claude Code CLI',
      'Claude Desktop',
      'Codex',
      'Gemini CLI',
      'Grok Build',
    ]) {
      expect(screen.getByRole('radio', { name })).toBeInTheDocument()
    }

    await user.click(screen.getByRole('button', { name: 'Other apps' }))

    expect(
      await screen.findByText(/MiniMax Code can't be imported by link/)
    ).toBeVisible()
  })

  it('shows the in-app preset steps and copies the key instead of opening CC Switch when Claude Desktop is selected', async () => {
    renderDialog()
    const user = userEvent.setup()

    await user.click(screen.getByRole('radio', { name: 'Claude Desktop' }))

    expect(
      screen.getByText(/Claude Desktop can't be imported by link/)
    ).toBeVisible()
    expect(screen.getByRole('alert')).toHaveTextContent(
      'Claude Desktop needs a key in a group that offers Claude models'
    )

    await user.click(screen.getByRole('button', { name: 'Copy Key' }))

    await expect(navigator.clipboard.readText()).resolves.toBe('sk-test-only')
    expect(window.open).not.toHaveBeenCalled()
  })

  it('keeps an existing sk- prefix when the key already has one', async () => {
    renderDialog({ tokenKey: 'sk-test-only' })
    const user = userEvent.setup()

    await user.click(screen.getByRole('button', { name: 'Open CC Switch' }))

    expect(openedImportParams().apiKey).toBe('sk-test-only')
  })
})

const storedKey: ApiKey = {
  id: 7,
  name: 'claude',
  key: 'abc',
  status: 1,
  remain_quota: -100,
  used_quota: 100,
  unlimited_quota: true,
  expired_time: -1,
  created_time: 1,
  accessed_time: 1,
  group: 'auto',
  auto_groups: ['vip'],
  cross_group_retry: true,
  model_limits_enabled: false,
  model_limits: '',
  allow_ips: '',
  allow_user_quota_query: false,
}

describe('CC Switch balance display', () => {
  let puts: Array<Record<string, unknown>>

  beforeEach(() => {
    puts = []
    vi.spyOn(api, 'put').mockImplementation(async (url, data) => {
      expect(url).toBe('/api/token/')
      puts.push(data as Record<string, unknown>)
      return { data: { success: true } }
    })
  })

  it('turns on user quota queries for the key and keeps its other settings when imported with the balance shown', async () => {
    const { onBalanceQueryEnabled } = renderDialog({ apiKey: storedKey })
    const user = userEvent.setup()

    expect(
      screen.getByRole('checkbox', {
        name: 'Show account balance in CC Switch',
      })
    ).toBeChecked()
    await user.click(screen.getByRole('button', { name: 'Open CC Switch' }))

    expect(openedImportParams().usageScript).toBeTruthy()
    await vi.waitFor(() => expect(onBalanceQueryEnabled).toHaveBeenCalled())
    expect(puts).toEqual([
      {
        id: 7,
        name: 'claude',
        remain_quota: -100,
        expired_time: -1,
        unlimited_quota: true,
        model_limits_enabled: false,
        model_limits: '',
        allow_ips: '',
        group: 'auto',
        cross_group_retry: true,
        allow_user_quota_query: true,
      },
    ])
  })

  it('imports without a usage script and leaves the key unchanged when the balance option is unchecked', async () => {
    renderDialog({ apiKey: storedKey })
    const user = userEvent.setup()

    await user.click(
      screen.getByRole('checkbox', {
        name: 'Show account balance in CC Switch',
      })
    )
    await user.click(screen.getByRole('button', { name: 'Open CC Switch' }))

    const params = openedImportParams()
    expect(params.usageScript).toBeUndefined()
    expect(params.usageEnabled).toBeUndefined()
    expect(puts).toHaveLength(0)
  })

  it('imports with the usage script without updating a key that already allows user quota queries', async () => {
    renderDialog({ apiKey: { ...storedKey, allow_user_quota_query: true } })
    const user = userEvent.setup()

    await user.click(screen.getByRole('button', { name: 'Open CC Switch' }))

    expect(openedImportParams().usageScript).toBeTruthy()
    expect(puts).toHaveLength(0)
  })

  it('hides the balance option when Claude Desktop is selected', async () => {
    renderDialog({ apiKey: storedKey })
    const user = userEvent.setup()

    await user.click(screen.getByRole('radio', { name: 'Claude Desktop' }))

    expect(
      screen.queryByRole('checkbox', {
        name: 'Show account balance in CC Switch',
      })
    ).not.toBeInTheDocument()
  })
})

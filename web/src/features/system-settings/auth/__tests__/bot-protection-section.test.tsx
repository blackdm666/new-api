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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { BotProtectionSection } from '../bot-protection-section'

const mutateAsync = vi.fn()

vi.mock('../../hooks/use-update-option', () => ({
  useUpdateBotProtectionSettings: () => ({
    mutateAsync,
    isPending: false,
  }),
}))

vi.mock('../../components/settings-page-context', async (importOriginal) => ({
  ...(await importOriginal<
    typeof import('../../components/settings-page-context')
  >()),
  SettingsPageFormActions: (props: { onSave: () => void }) => (
    <button type='button' onClick={() => props.onSave()}>
      Save bot protection
    </button>
  ),
}))

const customDefaults = {
  TurnstileCheckEnabled: true,
  TurnstileProvider: 'custom' as const,
  TurnstileSiteKey: '',
  TurnstileSecretKey: '',
  TurnstileSecretKeyConfigured: true,
  TurnstileWidgetScriptURL: 'https://captcha.example/widget.js',
  TurnstileWidgetEndpoint: 'https://captcha.example',
  TurnstileVerifyURL: 'https://captcha.example/turnstile/v0/siteverify',
  TurnstileAction: 'register',
}

describe('bot protection provider settings', () => {
  afterEach(() => mutateAsync.mockReset())

  test('saves custom slider settings in one request and preserves a blank secret', async () => {
    render(<BotProtectionSection defaultValues={customDefaults} />)

    fireEvent.change(
      screen.getByRole('textbox', { name: 'Widget script URL' }),
      {
        target: { value: 'https://slider.example/assets/widget.js' },
      }
    )
    fireEvent.change(
      screen.getByRole('textbox', { name: 'Widget API endpoint' }),
      { target: { value: 'https://slider.example/api/' } }
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save bot protection' }))

    await waitFor(() => expect(mutateAsync).toHaveBeenCalledTimes(1))
    expect(mutateAsync).toHaveBeenCalledWith({
      enabled: true,
      provider: 'custom',
      site_key: '',
      secret_key: '',
      widget_script_url: 'https://slider.example/assets/widget.js',
      widget_endpoint: 'https://slider.example/api/',
      verify_url: 'https://captcha.example/turnstile/v0/siteverify',
      action: 'register',
      clear_secret: false,
    })
  })

  test('rejects an unsafe custom verification URL before saving', async () => {
    render(<BotProtectionSection defaultValues={customDefaults} />)

    fireEvent.change(
      screen.getByRole('textbox', { name: 'Server verification URL' }),
      { target: { value: 'javascript:alert(1)' } }
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save bot protection' }))

    expect(await screen.findByText('Must be a valid URL')).toBeInTheDocument()
    expect(mutateAsync).not.toHaveBeenCalled()
  })

  test('shows Cloudflare fields without custom service URLs', () => {
    render(
      <BotProtectionSection
        defaultValues={{
          ...customDefaults,
          TurnstileProvider: 'cloudflare',
          TurnstileSiteKey: 'cloudflare-site-key',
          TurnstileWidgetScriptURL: '',
          TurnstileWidgetEndpoint: '',
          TurnstileVerifyURL: '',
        }}
      />
    )

    expect(screen.getByRole('textbox', { name: 'Site Key' })).toHaveValue(
      'cloudflare-site-key'
    )
    expect(
      screen.queryByRole('textbox', { name: 'Widget script URL' })
    ).not.toBeInTheDocument()
    expect(screen.getByPlaceholderText(/leave blank/i)).toHaveValue('')
  })

  test('can clear a stored secret while verification is disabled', async () => {
    render(
      <BotProtectionSection
        defaultValues={{
          ...customDefaults,
          TurnstileCheckEnabled: false,
        }}
      />
    )

    fireEvent.click(screen.getByRole('switch', { name: 'Clear stored secret' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save bot protection' }))

    await waitFor(() => expect(mutateAsync).toHaveBeenCalledTimes(1))
    expect(mutateAsync).toHaveBeenCalledWith(
      expect.objectContaining({
        enabled: false,
        secret_key: '',
        clear_secret: true,
      })
    )
  })

  test('allows an enabled custom service without a secret', async () => {
    render(
      <BotProtectionSection
        defaultValues={{
          ...customDefaults,
          TurnstileSecretKeyConfigured: false,
        }}
      />
    )

    expect(screen.getByLabelText('Secret Key (optional)')).toHaveValue('')
    fireEvent.click(screen.getByRole('button', { name: 'Save bot protection' }))

    await waitFor(() => expect(mutateAsync).toHaveBeenCalledTimes(1))
    expect(mutateAsync).toHaveBeenCalledWith(
      expect.objectContaining({
        enabled: true,
        provider: 'custom',
        secret_key: '',
      })
    )
  })
})

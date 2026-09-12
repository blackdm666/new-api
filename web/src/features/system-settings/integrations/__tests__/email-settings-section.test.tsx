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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import {
  EmailSettingsSection,
  type EmailFormValues,
} from '../email-settings-section'

const settings: EmailFormValues = {
  SMTPServer: 'smtp.primary.example',
  SMTPPort: '587',
  SMTPAccount: 'primary@example.com',
  SMTPFrom: 'primary@example.com',
  SMTPToken: '',
  SMTPSSLEnabled: false,
  SMTPStartTLSEnabled: true,
  SMTPInsecureSkipVerify: false,
  SMTPForceAuthLogin: false,
  SMTPBackupEnabled: false,
  SMTPBackupServer: 'smtp.backup.example',
  SMTPBackupPort: '465',
  SMTPBackupAccount: 'backup@example.com',
  SMTPBackupFrom: 'backup@example.com',
  SMTPBackupToken: '',
  SMTPBackupSSLEnabled: true,
  SMTPBackupStartTLSEnabled: false,
  SMTPBackupInsecureSkipVerify: false,
  SMTPBackupForceAuthLogin: false,
  SMTPSecurityEnabled: false,
  SMTPSecurityServer: 'smtp.security.example',
  SMTPSecurityPort: '465',
  SMTPSecurityAccount: 'security@example.com',
  SMTPSecurityFrom: 'security@example.com',
  SMTPSecurityToken: '',
  SMTPSecuritySSLEnabled: true,
  SMTPSecurityStartTLSEnabled: false,
  SMTPSecurityInsecureSkipVerify: false,
  SMTPSecurityForceAuthLogin: false,
  SMTPMarketingEnabled: false,
  SMTPMarketingServer: 'smtp.marketing.example',
  SMTPMarketingPort: '465',
  SMTPMarketingAccount: 'marketing@example.com',
  SMTPMarketingFrom: 'marketing@example.com',
  SMTPMarketingToken: '',
  SMTPMarketingSSLEnabled: true,
  SMTPMarketingStartTLSEnabled: false,
  SMTPMarketingInsecureSkipVerify: false,
  SMTPMarketingForceAuthLogin: false,
}

function renderSection(defaultValues = settings) {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <EmailSettingsSection defaultValues={defaultValues} />
    </QueryClientProvider>
  )
}

describe('SMTP settings', () => {
  afterEach(() => vi.restoreAllMocks())

  test('shows editable backup fields without a manual enable switch', () => {
    renderSection()

    fireEvent.click(screen.getByRole('tab', { name: 'Backup channel' }))
    const backupHost = screen.getByRole('textbox', { name: 'SMTP Host' })
    expect(backupHost).toBeEnabled()
    expect(
      screen.queryByRole('switch', { name: 'Enable backup SMTP channel' })
    ).not.toBeInTheDocument()
    expect(screen.getByText('Pending test')).toBeInTheDocument()
  })

  test('keeps marketing mail and the receipt interface as the final tabs', () => {
    renderSection()

    expect(screen.getAllByRole('tab').map((tab) => tab.textContent)).toEqual([
      'Security mail',
      'Notification mail',
      'Backup channel',
      'Marketing mail',
      'Receipt interface',
    ])
  })

  test('exposes enabled state for the shared green status style', () => {
    renderSection({ ...settings, SMTPSecurityEnabled: true })

    expect(screen.getByText('Enabled')).toHaveAttribute('data-state', 'enabled')
    expect(screen.getByText('Enabled')).toHaveClass('text-emerald-600')
  })

  test('shows verified marketing accounts and EventBridge setup separately', async () => {
    const put = vi.spyOn(api, 'put').mockResolvedValue({
      data: { success: true, message: '', data: {} },
    })
    vi.spyOn(api, 'get').mockImplementation((url) => {
      if (url === '/api/option/smtp/marketing-accounts') {
        return Promise.resolve({
          data: {
            success: true,
            message: '',
            data: [
              {
                id: 1,
                name: 'Alibaba sender A',
                profile: 'marketing',
                provider: 'aliyun_eventbridge',
                server: 'smtpdm.aliyun.com',
                port: 465,
                account: 'sender@example.com',
                from: 'sender@example.com',
                ssl_enabled: true,
                starttls_enabled: false,
                insecure_skip_verify: false,
                force_auth_login: false,
                weight: 2,
                rate_limit_per_minute: 20,
                enabled: true,
                tested_time: 1,
                receipt_verified_time: 1,
                disabled_until: 0,
                health_status: 'healthy',
                consecutive_failures: 0,
                last_success_time: 1,
                last_failure_time: 0,
                last_error: '',
                credential_configured: true,
              },
            ],
          },
        })
      }
      if (url === '/api/option/smtp/receipts') {
        return Promise.resolve({
          data: {
            success: true,
            message: '',
            data: {
              provider: 'aliyun_eventbridge',
              enabled: true,
              token_configured: true,
              callback_url:
                'https://example.com/api/email/receipts/aliyun/eventbridge',
              last_event_time: 1,
              last_verified_time: 1,
              last_error: '',
            },
          },
        })
      }
      throw new Error(`Unexpected GET ${url}`)
    })
    renderSection()

    fireEvent.click(screen.getByRole('tab', { name: 'Marketing mail' }))
    expect(await screen.findByText('Alibaba sender A')).toBeInTheDocument()
    expect(screen.getByText('1 / 1 enabled')).toHaveAttribute(
      'data-state',
      'enabled'
    )
    fireEvent.click(
      screen.getByRole('switch', {
        name: 'Enable marketing account Alibaba sender A',
      })
    )
    await waitFor(() =>
      expect(put).toHaveBeenCalledWith(
        '/api/option/smtp/marketing-accounts/1/enabled',
        { enabled: false }
      )
    )

    fireEvent.click(screen.getByRole('tab', { name: 'Receipt interface' }))
    expect(
      await screen.findByDisplayValue(
        'https://example.com/api/email/receipts/aliyun/eventbridge'
      )
    ).toBeInTheDocument()
    expect(
      screen.getByText(
        'Alibaba Cloud Direct Mail (EventBridge event distribution)'
      )
    ).toBeInTheDocument()
  })

  test('tests the dedicated security profile from the default tab', async () => {
    const post = vi.spyOn(api, 'post').mockResolvedValue({
      data: {
        success: true,
        message: '',
        data: {
          recipient: 'admin@example.com',
          profile: 'security',
          channel: 'security',
        },
      },
    })
    renderSection()

    expect(screen.getByRole('tab', { name: 'Security mail' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
    expect(screen.getByRole('textbox', { name: 'From Address' })).toHaveValue(
      'security@example.com'
    )
    fireEvent.click(
      screen.getByRole('button', { name: 'Test and enable selected profile' })
    )

    await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
    expect(post).toHaveBeenCalledWith(
      '/api/option/smtp-test',
      { email: '', channel: 'security' },
      expect.any(Object)
    )
  })

  test('sends a blank recipient so the server uses the current administrator email', async () => {
    const post = vi.spyOn(api, 'post').mockResolvedValue({
      data: {
        success: true,
        message: '',
        data: { recipient: 'admin@example.com', channel: 'primary' },
      },
    })
    renderSection()

    fireEvent.click(screen.getByRole('tab', { name: 'Notification mail' }))

    fireEvent.click(screen.getByRole('button', { name: 'Send test email' }))

    await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
    expect(post).toHaveBeenCalledWith(
      '/api/option/smtp-test',
      { email: '', channel: 'primary' },
      { skipBusinessError: true, skipErrorHandler: true }
    )
  })

  test('tests and activates the backup channel from its tab', async () => {
    const post = vi.spyOn(api, 'post').mockResolvedValue({
      data: {
        success: true,
        message: '',
        data: { recipient: 'admin@example.com', channel: 'backup' },
      },
    })
    renderSection()

    fireEvent.click(screen.getByRole('tab', { name: 'Backup channel' }))
    fireEvent.click(
      screen.getByRole('button', { name: 'Test and enable selected profile' })
    )

    await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
    expect(post).toHaveBeenCalledWith(
      '/api/option/smtp-test',
      { email: '', channel: 'backup' },
      { skipBusinessError: true, skipErrorHandler: true }
    )
    expect(screen.getByText('Enabled')).toBeInTheDocument()
  })

  test('saves edited backup settings before testing that channel', async () => {
    const put = vi.spyOn(api, 'put').mockResolvedValue({
      data: { success: true, message: '' },
    })
    const post = vi.spyOn(api, 'post').mockResolvedValue({
      data: {
        success: true,
        message: '',
        data: { recipient: 'admin@example.com', channel: 'backup' },
      },
    })
    renderSection()

    fireEvent.click(screen.getByRole('tab', { name: 'Backup channel' }))
    fireEvent.change(screen.getByRole('textbox', { name: 'SMTP Host' }), {
      target: { value: 'smtp.backup-updated.example' },
    })
    fireEvent.click(
      screen.getByRole('button', { name: 'Test and enable selected profile' })
    )

    await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
    expect(put).toHaveBeenCalledWith('/api/option/', {
      key: 'SMTPBackupServer',
      value: 'smtp.backup-updated.example',
    })
    expect(put.mock.invocationCallOrder[0]).toBeLessThan(
      post.mock.invocationCallOrder[0]
    )
  })
})

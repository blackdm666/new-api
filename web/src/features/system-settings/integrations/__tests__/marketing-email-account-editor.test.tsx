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
import { describe, expect, test, vi } from 'vitest'

import { MarketingEmailAccountEditor } from '../marketing-email-account-editor'

const validAccount = {
  name: 'Alibaba sender',
  provider: 'aliyun_eventbridge' as const,
  server: 'smtpdm.aliyun.com',
  port: 465,
  account: 'sender@example.com',
  from: 'sender@example.com',
  token: 'secret',
  ssl_enabled: true,
  starttls_enabled: false,
  insecure_skip_verify: false,
  force_auth_login: false,
  weight: 1,
  rate_limit_per_minute: 20,
}

describe('marketing email account editor', () => {
  test('requires a credential when a new account is saved', async () => {
    const onSave = vi.fn()
    render(
      <MarketingEmailAccountEditor
        initialValues={{ ...validAccount, token: '' }}
        editing={false}
        saving={false}
        onCancel={vi.fn()}
        onSave={onSave}
      />
    )

    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(
      await screen.findByText('Password or access token is required')
    ).toBeInTheDocument()
    expect(screen.getByLabelText('Password / Access Token')).toHaveAttribute(
      'aria-invalid',
      'true'
    )
    expect(onSave).not.toHaveBeenCalled()
  })

  test('keeps implicit TLS and STARTTLS mutually exclusive', () => {
    render(
      <MarketingEmailAccountEditor
        initialValues={validAccount}
        editing={false}
        saving={false}
        onCancel={vi.fn()}
        onSave={vi.fn()}
      />
    )

    expect(screen.getByRole('switch', { name: 'SSL/TLS' })).toBeChecked()
    fireEvent.click(screen.getByRole('switch', { name: 'STARTTLS' }))
    expect(screen.getByRole('switch', { name: 'STARTTLS' })).toBeChecked()
    expect(screen.getByRole('switch', { name: 'SSL/TLS' })).not.toBeChecked()
  })

  test('rejects generic SMTP accounts without delivery receipts', async () => {
    const onSave = vi.fn()
    render(
      <MarketingEmailAccountEditor
        initialValues={{ ...validAccount, server: 'smtp.qq.com' }}
        editing={false}
        saving={false}
        onCancel={vi.fn()}
        onSave={onSave}
      />
    )

    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(
      await screen.findByText(
        'Only Alibaba Cloud Direct Mail SMTP endpoints are supported'
      )
    ).toBeInTheDocument()
    expect(onSave).not.toHaveBeenCalled()
  })

  test('submits a complete validated account', async () => {
    const onSave = vi.fn()
    render(
      <MarketingEmailAccountEditor
        initialValues={validAccount}
        editing={false}
        saving={false}
        onCancel={vi.fn()}
        onSave={onSave}
      />
    )

    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(onSave).toHaveBeenCalledWith(validAccount))
  })
})

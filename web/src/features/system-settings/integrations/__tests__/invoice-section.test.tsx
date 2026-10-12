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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState, type ComponentProps } from 'react'
import { toast } from 'sonner'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { InvoiceSettingsSection } from '../invoice-section'

type DefaultValues = ComponentProps<
  typeof InvoiceSettingsSection
>['defaultValues']

const defaults: DefaultValues = {
  InvoiceApplicationNotifyAdminEnabled: true,
  InvoiceIssuedNotifyUserEnabled: true,
  InvoiceRejectedNotifyUserEnabled: true,
  InvoiceExpiredNotifyUserEnabled: true,
  InvoiceWechatNotifyEnabled: false,
  InvoiceWechatApiBaseUrl: 'http://api.wechatapi.net/finder/v2/api',
  InvoiceWechatApiToken: 'stored-token-must-not-render',
  InvoiceWechatAppId: 'wx-app',
  InvoiceWechatGroupId: '12345678901@chatroom',
  InvoiceAdminEmail: 'billing@example.com',
  InvoiceMinimumAmountCents: '50000',
  InvoiceTaxRateBasisPoints: '300',
  InvoiceDataRetentionDays: '0',
  InvoicePendingExpiryDays: '30',
  InvoiceFileEnabled: true,
  InvoiceFileStorage: 'local',
  InvoiceFileMaxSize: '5242880',
  InvoiceFileMaxCount: '5',
  InvoiceFileAllowedExts: 'pdf',
  InvoiceFileLocalPath: '/data/invoice_files',
  InvoiceFileSignedURLTTL: '900',
  InvoiceFileOSSEndpoint: '',
  InvoiceFileOSSBucket: '',
  InvoiceFileOSSRegion: '',
  InvoiceFileOSSAccessKeyId: '',
  InvoiceFileOSSAccessKeySecret: '',
  InvoiceFileOSSCustomDomain: '',
  InvoiceFileS3Endpoint: '',
  InvoiceFileS3Bucket: '',
  InvoiceFileS3Region: '',
  InvoiceFileS3AccessKeyId: '',
  InvoiceFileS3AccessKeySecret: '',
  InvoiceFileS3CustomDomain: '',
  InvoiceFileCOSEndpoint: '',
  InvoiceFileCOSBucket: '',
  InvoiceFileCOSRegion: '',
  InvoiceFileCOSSecretId: '',
  InvoiceFileCOSSecretKey: '',
  InvoiceFileCOSCustomDomain: '',
}

function Fixture(props: { defaultValues: DefaultValues }) {
  const [container, setContainer] = useState<HTMLDivElement | null>(null)
  return (
    <>
      <div ref={setContainer} />
      <SettingsPageProvider actionsContainer={container}>
        <InvoiceSettingsSection defaultValues={props.defaultValues} />
      </SettingsPageProvider>
    </>
  )
}

function renderSection(defaultValues: DefaultValues = defaults) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <Fixture defaultValues={defaultValues} />
    </QueryClientProvider>
  )
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('invoice settings notifications', () => {
  test('saving sends the rejected and expired user notification switches', async () => {
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true, message: '' } })
    const user = userEvent.setup()
    renderSection()

    await user.click(
      screen.getByRole('switch', {
        name: 'Notify user when an invoice application is rejected',
      })
    )
    await user.click(
      await screen.findByRole('button', { name: 'Save invoice settings' })
    )

    await waitFor(() => expect(put).toHaveBeenCalledOnce())
    expect(put.mock.calls[0][1]).toMatchObject({
      InvoiceRejectedNotifyUserEnabled: false,
      InvoiceExpiredNotifyUserEnabled: true,
    })
  })

  test('the stored WeChat token is never shown and a blank token is sent to keep it', async () => {
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true, message: '' } })
    const user = userEvent.setup()
    renderSection({ ...defaults, InvoiceWechatApiTokenConfigured: true })

    const token = screen.getByLabelText('WeChat API token')
    expect(token).toHaveValue('')
    expect(token).toHaveAttribute('type', 'password')
    expect(screen.getByText('A token is saved.')).toBeVisible()

    await user.click(
      screen.getByRole('switch', { name: 'Enable WeChat group notifications' })
    )
    await user.click(
      await screen.findByRole('button', { name: 'Save invoice settings' })
    )

    await waitFor(() => expect(put).toHaveBeenCalledOnce())
    expect(put.mock.calls[0][1]).toMatchObject({
      InvoiceWechatNotifyEnabled: true,
      InvoiceWechatApiToken: '',
      InvoiceWechatAppId: 'wx-app',
      InvoiceWechatGroupId: '12345678901@chatroom',
    })
  })

  test('enabling WeChat notifications without a saved or entered token blocks saving', async () => {
    const put = vi.spyOn(api, 'put')
    const error = vi.spyOn(toast, 'error')
    const user = userEvent.setup()
    renderSection({ ...defaults, InvoiceWechatApiTokenConfigured: false })

    expect(screen.getAllByText('No token saved yet').length).toBeGreaterThan(0)
    await user.click(
      screen.getByRole('switch', { name: 'Enable WeChat group notifications' })
    )
    await user.click(
      await screen.findByRole('button', { name: 'Save invoice settings' })
    )

    expect(error).toHaveBeenCalledWith(
      'Enter the WeChat API token before enabling WeChat group notifications.'
    )
    expect(put).not.toHaveBeenCalled()
  })

  test('a WeChat group ID without the @chatroom suffix blocks saving', async () => {
    const put = vi.spyOn(api, 'put')
    const user = userEvent.setup()
    renderSection()

    const groupId = screen.getByLabelText('WeChat group ID')
    await user.clear(groupId)
    await user.type(groupId, '12345678901')

    expect(groupId).toHaveAttribute('aria-invalid', 'true')
    expect(
      screen.getByText('The WeChat group ID must end with @chatroom.')
    ).toBeVisible()

    await user.click(
      await screen.findByRole('button', { name: 'Save invoice settings' })
    )
    expect(put).not.toHaveBeenCalled()
  })

  test('sending a WeChat test message shows the message returned by the server', async () => {
    const post = vi.spyOn(api, 'post').mockResolvedValue({
      data: { success: false, message: 'Group 12345678901@chatroom not found' },
    })
    const error = vi.spyOn(toast, 'error')
    const user = userEvent.setup()
    renderSection()

    await user.click(screen.getByRole('button', { name: 'Send test message' }))

    await waitFor(() =>
      expect(error).toHaveBeenCalledWith('Group 12345678901@chatroom not found')
    )
    expect(post).toHaveBeenCalledWith(
      '/api/invoice/admin/wechat/test',
      undefined,
      expect.anything()
    )
  })
})

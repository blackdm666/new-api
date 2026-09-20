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
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { UserNotificationWorkbench } from '..'
import type { NoticeRecord, NoticeUser } from '../types'

const users: NoticeUser[] = [
  {
    id: 1001,
    username: 'normal',
    display_name: 'Normal user',
    email_masked: 'n***@example.invalid',
    group: 'default',
    disabled: false,
    skip_reason: null,
  },
  {
    id: 1004,
    username: 'disabled',
    display_name: 'Disabled user',
    email_masked: 'd***@example.invalid',
    group: 'default',
    disabled: true,
    skip_reason: null,
  },
  {
    id: 1005,
    username: 'missing',
    display_name: 'Missing email',
    email_masked: '',
    group: 'default',
    disabled: false,
    skip_reason: 'missing_email',
  },
  {
    id: 1006,
    username: 'blocked',
    display_name: 'Blocked email',
    email_masked: 'b***@example.invalid',
    group: 'default',
    disabled: false,
    skip_reason: 'blocked_email',
  },
]

function response(data: unknown): ReturnType<typeof api.get> {
  return Promise.resolve({ data: { success: true, data } }) as ReturnType<
    typeof api.get
  >
}

function renderComposer(records: NoticeRecord[] = [], preview = true) {
  vi.spyOn(api, 'get').mockImplementation((url, config) => {
    if (url.endsWith('/notices') || url.endsWith('/user-notices')) {
      return response(records)
    }
    if (url.endsWith('/users')) {
      return response(
        users.filter((user) => user.username.includes(config?.params.q ?? ''))
      )
    }
    throw new Error(`Unexpected preview endpoint: ${url}`)
  })
  const post = vi
    .spyOn(api, 'post')
    .mockRejectedValue(new Error('Local service unavailable'))
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <UserNotificationWorkbench preview={preview} />
    </QueryClientProvider>
  )
  return post
}

describe('targeted notification composer', () => {
  test('production confirmation uses the real endpoint and preserves the request key after a network failure', async () => {
    const user = userEvent.setup()
    const post = renderComposer([], false)
    await user.click(await screen.findByRole('checkbox', { name: /@normal/ }))
    await user.click(screen.getByRole('button', { name: 'Use template' }))
    await user.click(screen.getByRole('button', { name: 'Review and send' }))
    expect(
      await screen.findByText(
        'This will send real service emails to the selected users using verification SMTP first. Check the content and recipients carefully.'
      )
    ).toBeInTheDocument()
    await user.click(
      screen.getByRole('button', { name: 'Confirm and queue notification' })
    )
    await screen.findByText('Could not save notification')
    await user.click(screen.getByRole('button', { name: 'Review and send' }))
    await user.click(
      screen.getByRole('button', { name: 'Confirm and queue notification' })
    )
    await waitFor(() => expect(post).toHaveBeenCalledTimes(2))
    expect(post.mock.calls[0][0]).toBe('/api/marketing/user-notices')
    expect(post.mock.calls[1][1]).toEqual(post.mock.calls[0][1])
    expect(
      screen.queryByText('Local interactive preview — no emails will be sent')
    ).not.toBeInTheDocument()
  })

  test('production details distinguish SMTP acceptance from inbox delivery and show the actual fallback', async () => {
    const user = userEvent.setup()
    const record: NoticeRecord = {
      id: 9,
      kind: 'usage',
      subject: 'API reminder',
      body: 'Check request parameters.',
      recipients: [
        {
          ...users[0],
          state: 'accepted_untracked',
          smtp_channel: 'primary',
          smtp_profile: 'security',
        },
      ],
      status: 'queued',
      created_at: 1,
      operator: 'UID 1',
    }
    renderComposer([record], false)
    await user.click(await screen.findByRole('button', { name: 'Details' }))
    const detail = await screen.findByRole('dialog')
    expect(
      within(detail).getByText('SMTP accepted; inbox delivery unconfirmed')
    ).toBeInTheDocument()
    expect(
      within(detail).getByText('Primary SMTP fallback')
    ).toBeInTheDocument()
    expect(within(detail).queryByText('Delivered')).not.toBeInTheDocument()
  })

  test('requires an explicit eligible recipient even when content is complete', async () => {
    const user = userEvent.setup()
    renderComposer()
    await user.click(screen.getByRole('button', { name: 'Use template' }))
    expect(
      screen.getByRole('button', { name: 'Review and send' })
    ).toBeDisabled()
    await user.click(await screen.findByRole('checkbox', { name: /@missing/ }))
    await user.click(screen.getByRole('checkbox', { name: /@blocked/ }))
    expect(
      screen.getByRole('button', { name: 'Review and send' })
    ).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Save draft' })).toBeDisabled()
  })

  test('retains selected recipients across searches and supports disabled accounts', async () => {
    const user = userEvent.setup()
    renderComposer()
    await user.click(await screen.findByRole('checkbox', { name: /@disabled/ }))
    await user.type(
      screen.getByRole('textbox', { name: 'Find recipients' }),
      'normal'
    )
    await waitFor(() =>
      expect(
        screen.queryByRole('checkbox', { name: /@disabled/ })
      ).not.toBeInTheDocument()
    )
    expect(
      screen.getByRole('button', { name: 'Remove disabled' })
    ).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Use template' }))
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Review and send' })
      ).toBeEnabled()
    )
  })

  test('canceling review never submits a notification and preserves content', async () => {
    const user = userEvent.setup()
    const post = renderComposer()
    await user.click(await screen.findByRole('checkbox', { name: /@normal/ }))
    await user.click(screen.getByRole('button', { name: 'Use template' }))
    await user.click(screen.getByRole('button', { name: 'Review and send' }))
    const dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    expect(post).not.toHaveBeenCalled()
    expect(screen.getByRole('textbox', { name: 'Subject' })).toHaveValue(
      'Scheduled service maintenance'
    )
  })

  test('confirmation submits only selected IDs and records simulation without claiming delivery', async () => {
    const user = userEvent.setup()
    const records: NoticeRecord[] = []
    const post = renderComposer(records)
    const record: NoticeRecord = {
      id: 1,
      kind: 'maintenance',
      subject: 'Scheduled service maintenance',
      body: 'Maintenance notice example',
      recipients: [users[1], users[2]],
      status: 'simulated',
      created_at: 1,
      operator: 'Preview admin',
    }
    post.mockImplementation(() => {
      records.push(record)
      return response(record)
    })
    await user.click(await screen.findByRole('checkbox', { name: /@disabled/ }))
    await user.click(screen.getByRole('checkbox', { name: /@missing/ }))
    await user.click(screen.getByRole('button', { name: 'Use template' }))
    await user.click(screen.getByRole('button', { name: 'Review and send' }))
    await user.click(
      await screen.findByRole('button', { name: 'Confirm simulated send' })
    )
    await screen.findByText('Simulated submission')
    expect(post).toHaveBeenCalledExactlyOnceWith(
      '/__notification-preview/notices',
      expect.objectContaining({
        user_ids: [1004, 1005],
        action: 'send',
        request_key: expect.any(String),
      })
    )
    await user.click(screen.getByRole('button', { name: 'Details' }))
    const detail = await screen.findByRole('dialog')
    expect(
      within(detail).getByText('Simulated only; not delivered')
    ).toBeInTheDocument()
    expect(
      within(detail).getByText('Skipped: no email address')
    ).toBeInTheDocument()
  })

  test('failed draft save retains the selected user and message for retry', async () => {
    const user = userEvent.setup()
    renderComposer()
    await user.click(await screen.findByRole('checkbox', { name: /@normal/ }))
    await user.click(screen.getByRole('button', { name: 'Use template' }))
    await user.click(screen.getByRole('button', { name: 'Save draft' }))
    await screen.findByText('Could not save notification')
    expect(screen.getByRole('textbox', { name: 'Subject' })).toHaveValue(
      'Scheduled service maintenance'
    )
    expect(screen.getByRole('checkbox', { name: /@normal/ })).toBeChecked()
  })

  test('continuing a draft restores recipients and updates that draft', async () => {
    const user = userEvent.setup()
    const draft: NoticeRecord = {
      id: 8,
      kind: 'usage',
      subject: 'Review request rate',
      body: 'Please reduce concurrency.',
      recipients: [users[0]],
      status: 'draft',
      created_at: 1,
      operator: 'Preview admin',
    }
    const post = renderComposer([draft])
    post.mockResolvedValue(await response(draft))
    await user.click(
      await screen.findByRole('button', { name: 'Continue editing' })
    )
    expect(screen.getByRole('textbox', { name: 'Subject' })).toHaveValue(
      'Review request rate'
    )
    expect(screen.getByRole('checkbox', { name: /@normal/ })).toBeChecked()
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Save draft' })).toBeEnabled()
    )
    await user.click(screen.getByRole('button', { name: 'Save draft' }))
    await screen.findByText(
      'Local draft saved. You can return to it from notification history.'
    )
    expect(post).toHaveBeenCalledWith(
      '/__notification-preview/notices',
      expect.objectContaining({ id: 8, user_ids: [1001], action: 'draft' })
    )
  })

  test('whitespace content is invalid and literal HTML is rendered as text', async () => {
    const user = userEvent.setup()
    renderComposer()
    await user.click(await screen.findByRole('checkbox', { name: /@normal/ }))
    fireEvent.change(screen.getByRole('textbox', { name: 'Subject' }), {
      target: { value: '   ' },
    })
    fireEvent.change(screen.getByRole('textbox', { name: 'Message' }), {
      target: { value: '<img src=x onerror=alert(1)>' },
    })
    expect(
      screen.getByRole('button', { name: 'Review and send' })
    ).toBeDisabled()
    expect(
      screen.getByText('<img src=x onerror=alert(1)>', { selector: 'p' })
    ).toBeInTheDocument()
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
  })
})

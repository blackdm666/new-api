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
import { useState } from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { ReferralApplicationSection } from '../referral-application-section'

function Fixture(props: { recipient: string }) {
  const [container, setContainer] = useState<HTMLDivElement | null>(null)
  return (
    <>
      <div ref={setContainer} />
      <SettingsPageProvider actionsContainer={container}>
        <ReferralApplicationSection
          defaultValues={{ ReferralApplicationRecipient: props.recipient }}
        />
      </SettingsPageProvider>
    </>
  )
}

function renderSection(recipient = 'laohei@88fk.org') {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <Fixture recipient={recipient} />
    </QueryClientProvider>
  )
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('referral application settings', () => {
  test('saving normalizes several recipients into one comma separated option', async () => {
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true, message: '' } })
    const user = userEvent.setup()
    renderSection()

    const input = screen.getByLabelText('Application recipient email')
    await user.clear(input)
    await user.type(input, 'ops@example.com;  sales@example.com')
    await user.click(
      await screen.findByRole('button', {
        name: 'Save referral application settings',
      })
    )

    await waitFor(() => expect(put).toHaveBeenCalledOnce())
    expect(put.mock.calls[0][1]).toEqual({
      key: 'ReferralApplicationRecipient',
      value: 'ops@example.com, sales@example.com',
    })
  })

  test('an invalid address shows a field error and is not saved', async () => {
    const put = vi.spyOn(api, 'put')
    const user = userEvent.setup()
    renderSection()

    const input = screen.getByLabelText('Application recipient email')
    await user.clear(input)
    await user.type(input, 'ops@example.com, not-an-email')
    await user.click(
      await screen.findByRole('button', {
        name: 'Save referral application settings',
      })
    )

    expect(
      await screen.findByText(
        'Enter one or more valid email addresses separated by commas'
      )
    ).toBeVisible()
    expect(input).toHaveAttribute('aria-invalid', 'true')
    expect(put).not.toHaveBeenCalled()
  })
})

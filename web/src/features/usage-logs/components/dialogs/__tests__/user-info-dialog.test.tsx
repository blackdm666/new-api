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
import { render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, describe, expect, test } from 'vitest'

import { formatTimestampToDate } from '@/lib/format'

const i18n = (await import('i18next')).default
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { api } = await import('@/lib/api')
const { UserInfoDialog } = await import('@/components/user-info-dialog')

await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

type ApiGet = (url: string) => Promise<{ data: unknown }>
type MockableApi = { get: ApiGet }

const apiClient = api as unknown as MockableApi
const originalGet = apiClient.get

function renderDialog(userId = 42) {
  return render(
    <I18nextProvider i18n={i18n}>
      <UserInfoDialog userId={userId} open onOpenChange={() => undefined} />
    </I18nextProvider>
  )
}

function infoItem(label: string): HTMLElement {
  const item = screen.getByText(label).parentElement
  if (!item) throw new Error(`Expected an info item for ${label}`)
  return item
}

afterEach(() => {
  apiClient.get = originalGet
})

describe('usage log user information dialog', () => {
  test('shows account metadata and the actual inviter instead of the invitation code', async () => {
    const createdAt = 1_786_700_100
    const lastLoginAt = 1_786_700_200
    apiClient.get = async (url) => {
      expect(url).toBe('/api/user/42')
      return {
        data: {
          success: true,
          data: {
            id: 42,
            username: 'usage-user',
            display_name: 'Usage User',
            created_at: createdAt,
            last_login_at: lastLoginAt,
            quota: 500_000,
            used_quota: 250_000,
            request_count: 12,
            group: 'default',
            inviter_id: 7,
            inviter_username: 'promoter-user',
            aff_code: 'OWN-CODE',
            aff_count: 3,
            aff_quota: 0,
            remark: 'priority customer',
          },
        },
      }
    }

    renderDialog()
    await waitFor(() => expect(screen.getByText('usage-user')).toBeVisible())

    expect(within(infoItem('User ID')).getByText('42')).toBeVisible()
    expect(infoItem('Created At')).toHaveTextContent(
      formatTimestampToDate(createdAt)
    )
    expect(infoItem('Last Login')).toHaveTextContent(
      formatTimestampToDate(lastLoginAt)
    )
    expect(infoItem('Remark')).toHaveTextContent('priority customer')
    expect(infoItem('Inviter')).toHaveTextContent('promoter-user (ID: 7)')
    expect(infoItem('Invited User Count')).toHaveTextContent('3')
    expect(screen.queryByText('Invitation Code')).not.toBeInTheDocument()
    expect(screen.queryByText('OWN-CODE')).not.toBeInTheDocument()
  })

  test('keeps metadata fields visible when optional values are empty', async () => {
    apiClient.get = async () => ({
      data: {
        success: true,
        data: {
          id: 42,
          username: 'new-user',
          created_at: 1_786_700_100,
          last_login_at: 0,
          quota: 0,
          used_quota: 0,
          request_count: 0,
          group: 'default',
          inviter_id: 0,
          aff_count: 0,
          remark: '',
        },
      },
    })

    renderDialog()
    await waitFor(() => expect(screen.getByText('new-user')).toBeVisible())

    expect(infoItem('Display Name')).toHaveTextContent('Not set')
    expect(infoItem('Last Login')).toHaveTextContent('Never logged in')
    expect(infoItem('Remark')).toHaveTextContent('Not set')
    expect(infoItem('Inviter')).toHaveTextContent('None')
    expect(infoItem('Invited User Count')).toHaveTextContent('0')
  })

  test('does not show stale information when users are switched quickly', async () => {
    let resolveFirstRequest: ((value: { data: unknown }) => void) | undefined
    apiClient.get = (url) => {
      if (url === '/api/user/42') {
        return new Promise((resolve) => {
          resolveFirstRequest = resolve
        })
      }
      expect(url).toBe('/api/user/43')
      return Promise.resolve({
        data: {
          success: true,
          data: {
            id: 43,
            username: 'current-user',
            created_at: 1_786_700_100,
            last_login_at: 0,
            quota: 0,
            used_quota: 0,
            request_count: 0,
            group: 'default',
            inviter_id: 0,
            aff_count: 0,
          },
        },
      })
    }

    const view = renderDialog(42)
    view.rerender(
      <I18nextProvider i18n={i18n}>
        <UserInfoDialog userId={43} open onOpenChange={() => undefined} />
      </I18nextProvider>
    )

    await waitFor(() => expect(screen.getByText('current-user')).toBeVisible())
    resolveFirstRequest?.({
      data: {
        success: true,
        data: {
          id: 42,
          username: 'stale-user',
        },
      },
    })

    await waitFor(() =>
      expect(screen.queryByText('stale-user')).not.toBeInTheDocument()
    )
    expect(screen.getByText('current-user')).toBeVisible()
  })
})

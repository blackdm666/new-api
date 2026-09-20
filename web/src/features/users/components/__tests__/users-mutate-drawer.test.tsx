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
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  type RenderResult,
} from '@testing-library/react'
import { afterEach, describe, expect, test } from 'vitest'

import type { User } from '../../types'

const i18n = (await import('i18next')).default
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { QueryClient, QueryClientProvider } =
  await import('@tanstack/react-query')
const { api } = await import('@/lib/api')
const { UsersProvider } = await import('../users-provider')
const { UsersMutateDrawer } = await import('../users-mutate-drawer')

await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

type ApiMethod = (url: string, data?: unknown) => Promise<{ data: unknown }>
type MockableApi = {
  get: ApiMethod
  put: ApiMethod
}

const apiClient = api as unknown as MockableApi
const originalGet = apiClient.get
const originalPut = apiClient.put
let renderedDrawer: RenderResult | null = null
let queryClient: InstanceType<typeof QueryClient> | null = null

function user(id: number, displayName: string, inviterId: number): User {
  return {
    id,
    username: `user-${id}`,
    display_name: displayName,
    quota: id * 500000,
    used_quota: 0,
    request_count: 0,
    group: 'default',
    inviter_id: inviterId,
    status: 1,
    role: 1,
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((promiseResolve) => {
    resolve = promiseResolve
  })
  return { promise, resolve }
}

function drawerTree(currentRow: User) {
  if (!queryClient) throw new Error('Expected a query client')

  return (
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={queryClient}>
        <UsersProvider>
          <UsersMutateDrawer
            open
            currentRow={currentRow}
            onOpenChange={() => undefined}
          />
        </UsersProvider>
      </QueryClientProvider>
    </I18nextProvider>
  )
}

function getSubmitButton(): HTMLButtonElement {
  const button = document.querySelector<HTMLButtonElement>(
    'button[form="user-form"]'
  )
  if (!button) throw new Error('Expected the user form submit button')
  return button
}

afterEach(() => {
  apiClient.get = originalGet
  apiClient.put = originalPut
  renderedDrawer = null
  queryClient?.clear()
  queryClient = null
})

describe('user drawer', () => {
  test('ignores an older detail response after switching users', async () => {
    const first = user(1, 'First User', 11)
    const second = user(2, 'Second User', 22)
    const firstRequest = deferred<{ data: unknown }>()
    const secondRequest = deferred<{ data: unknown }>()
    const requestedUrls: string[] = []
    const updates: Array<Record<string, unknown>> = []

    apiClient.get = (url) => {
      requestedUrls.push(url)
      if (url === '/api/user/1') return firstRequest.promise
      if (url === '/api/user/2') return secondRequest.promise
      if (url === '/api/group/') {
        return Promise.resolve({
          data: { success: true, data: ['default'] },
        })
      }
      if (url === '/api/authz/catalog') {
        return Promise.resolve({
          data: { success: true, data: { resources: [], roles: [] } },
        })
      }
      if (url === '/api/user/inviter-options') {
        return Promise.resolve({ data: { success: true, data: [] } })
      }
      throw new Error(`Unexpected GET ${url}`)
    }
    apiClient.put = async (_url, data) => {
      if (!data || typeof data !== 'object') {
        throw new Error('Expected an update payload')
      }
      updates.push(data as Record<string, unknown>)
      return { data: { success: true, data: second } }
    }

    queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    renderedDrawer = render(drawerTree(first))
    expect(getSubmitButton()).toBeDisabled()
    expect(document.querySelector('#user-form')).toHaveAttribute('inert')

    renderedDrawer.rerender(drawerTree(second))
    await waitFor(() => expect(requestedUrls).toContain('/api/user/2'))

    await act(async () => {
      secondRequest.resolve({ data: { success: true, data: second } })
      await secondRequest.promise
    })
    await waitFor(() => expect(getSubmitButton()).toBeEnabled())
    expect(document.querySelector('#user-form')).not.toHaveAttribute('inert')
    expect(screen.getByLabelText('Display Name')).toHaveValue('Second User')

    await act(async () => {
      firstRequest.resolve({ data: { success: true, data: first } })
      await firstRequest.promise
    })
    expect(screen.getByLabelText('Display Name')).toHaveValue('Second User')

    fireEvent.change(screen.getByLabelText('Display Name'), {
      target: { value: 'Second User Updated' },
    })
    fireEvent.submit(document.querySelector('#user-form') as HTMLFormElement)
    await waitFor(() => expect(updates).toHaveLength(1))

    expect(updates[0]?.id).toBe(2)
    expect(updates[0]?.display_name).toBe('Second User Updated')
    expect(updates[0]?.inviter_id).toBe(22)
  })
})

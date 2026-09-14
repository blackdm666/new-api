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
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { act, cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AxiosError } from 'axios'
import { toast } from 'sonner'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { STATUS_QUERY_KEY } from '@/lib/status-query'
import { useAuthStore } from '@/stores/auth-store'

import { UserAuthForm } from '../user-auth-form'

let client: QueryClient
const originalTurnstile = window.turnstile

beforeEach(() => {
  client = new QueryClient({
    defaultOptions: { queries: { enabled: false, retry: false } },
  })
  client.setQueryData(STATUS_QUERY_KEY, {
    password_login_enabled: true,
    turnstile_check: false,
  })
  useAuthStore.getState().auth.reset('complete')
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
})

afterEach(() => {
  cleanup()
  client.clear()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  window.turnstile = originalTurnstile
  useAuthStore.getState().auth.reset('idle')
})

it('requires a fresh CAPTCHA after a failed password attempt and reports each manual attempt once', async () => {
  client.setQueryData(STATUS_QUERY_KEY, {
    password_login_enabled: true,
    turnstile_check: true,
    turnstile_site_key: 'test-site',
  })
  let verify!: (token: string) => void
  window.turnstile = {
    render: (_element, options) => {
      verify = options.callback as (token: string) => void
      return 'widget'
    },
    remove: () => {},
  }
  const post = vi.spyOn(api, 'post').mockImplementation(async () => ({
    data: { success: false, message: '用户名或密码错误' },
  }))
  const errors = vi.spyOn(toast, 'error')
  const router = createRouter({
    routeTree: createRootRoute({ component: UserAuthForm }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  const user = userEvent.setup()
  await user.type(
    await screen.findByLabelText('Username or Email'),
    'test-user'
  )
  await user.type(screen.getByLabelText('Password'), 'wrong-password')
  const submit = screen.getByRole('button', { name: 'Sign in' })
  expect(submit).toBeDisabled()
  act(() => verify('one-use-token'))
  await user.click(submit)
  await waitFor(() => expect(errors).toHaveBeenCalledTimes(1))
  expect(submit).toBeDisabled()
  expect(post).toHaveBeenCalledTimes(1)
  act(() => verify('new-one-use-token'))
  await user.click(submit)
  await waitFor(() => expect(errors).toHaveBeenCalledTimes(2))
  expect(post).toHaveBeenCalledTimes(2)
  expect(post.mock.calls[1][0]).toContain('turnstile=new-one-use-token')
})

it('shows a safe stable-code error after WeChat login fails', async () => {
  client.setQueryData(STATUS_QUERY_KEY, {
    password_login_enabled: true,
    wechat_login: true,
  })
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: false,
      code: 'AUTH_INTERNAL_ERROR',
      message: 'private backend diagnostic',
    },
  })
  const errors = vi.spyOn(toast, 'error')
  const router = createRouter({
    routeTree: createRootRoute({ component: UserAuthForm }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  const user = userEvent.setup()
  await user.click(
    await screen.findByRole('button', { name: /Continue with WeChat/ })
  )
  await user.type(screen.getByLabelText('Verification code'), 'test-code')
  await user.click(screen.getByRole('button', { name: 'Confirm' }))
  await waitFor(() =>
    expect(errors).toHaveBeenCalledWith('Please try again later.')
  )
  expect(errors).toHaveBeenCalledTimes(1)
  expect(useAuthStore.getState().auth.user).toBeNull()
})

it('reports a safe Passkey begin failure without invoking the authenticator or replaying', async () => {
  client.setQueryData(STATUS_QUERY_KEY, {
    password_login_enabled: true,
    passkey_login: true,
  })
  vi.stubGlobal('PublicKeyCredential', class {})
  const getCredential = vi.fn()
  const credentials = Object.getOwnPropertyDescriptor(navigator, 'credentials')
  Object.defineProperty(navigator, 'credentials', {
    configurable: true,
    value: { get: getCredential },
  })
  try {
    const post = vi.spyOn(api, 'post').mockResolvedValue({
      data: {
        success: false,
        code: 'AUTH_INTERNAL_ERROR',
        message: 'private backend diagnostic',
      },
    })
    const errors = vi.spyOn(toast, 'error')
    const router = createRouter({
      routeTree: createRootRoute({ component: UserAuthForm }),
      history: createMemoryHistory({ initialEntries: ['/'] }),
    })
    render(
      <QueryClientProvider client={client}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    )
    const button = await screen.findByRole('button', {
      name: 'Sign in with Passkey',
    })
    await waitFor(() => expect(button).toBeEnabled())
    await userEvent.setup().click(button)
    await waitFor(() =>
      expect(errors).toHaveBeenCalledWith('Please try again later.')
    )
    expect(errors).toHaveBeenCalledTimes(1)
    expect(post).toHaveBeenCalledTimes(1)
    expect(post.mock.calls[0][0]).toBe('/api/user/passkey/login/begin')
    expect(getCredential).not.toHaveBeenCalled()
    expect(useAuthStore.getState().auth.user).toBeNull()
    expect(button).toBeEnabled()
  } finally {
    if (credentials) {
      Object.defineProperty(navigator, 'credentials', credentials)
    } else Reflect.deleteProperty(navigator, 'credentials')
  }
})

it.each(['business', 'http', 'network'] as const)(
  'shows a safe error after a %s login failure without authenticating or replaying',
  async (kind) => {
    const failure = {
      success: false,
      code: 'AUTH_INVALID_CREDENTIALS',
      message: '用户名或密码错误',
    }
    const post = vi.spyOn(api, 'post')
    if (kind === 'business') post.mockResolvedValue({ data: failure })
    else if (kind === 'http') {
      post.mockRejectedValue(
        new AxiosError(
          'Request failed',
          'ERR_BAD_REQUEST',
          undefined,
          undefined,
          {
            data: failure,
            status: 400,
            statusText: 'Bad Request',
            headers: {},
            config: {} as never,
          }
        )
      )
    } else {
      post.mockRejectedValue(new AxiosError('Network Error', 'ERR_NETWORK'))
    }
    const errorToast = vi.spyOn(toast, 'error')
    const router = createRouter({
      routeTree: createRootRoute({ component: UserAuthForm }),
      history: createMemoryHistory({ initialEntries: ['/'] }),
    })
    render(
      <QueryClientProvider client={client}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    )
    const user = userEvent.setup()
    await user.type(
      await screen.findByLabelText('Username or Email'),
      'test-user'
    )
    await user.type(screen.getByLabelText('Password'), 'wrong-password')
    await user.click(screen.getByRole('button', { name: 'Sign in' }))
    await waitFor(() => expect(errorToast).toHaveBeenCalledTimes(1))
    expect(errorToast).toHaveBeenCalledWith(
      kind === 'network' ? 'Network Error' : '用户名或密码错误'
    )
    expect(post).toHaveBeenCalledTimes(1)
    expect(post.mock.calls[0]?.[2]).toMatchObject({ skipAuthRefresh: true })
    expect(useAuthStore.getState().auth.user).toBeNull()
    expect(screen.getByRole('button', { name: 'Sign in' })).toBeEnabled()
  }
)

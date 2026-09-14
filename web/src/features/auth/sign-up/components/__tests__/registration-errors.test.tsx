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
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AxiosError } from 'axios'
import { toast } from 'sonner'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { STATUS_QUERY_KEY } from '@/lib/status-query'

import { SignUpForm } from '../sign-up-form'

let client: QueryClient
beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { enabled: false } } })
  client.setQueryData(STATUS_QUERY_KEY, { turnstile_check: false })
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
})
afterEach(() => {
  cleanup()
  client.clear()
  vi.restoreAllMocks()
})

it.each(['register', 'send code'] as const)(
  'shows a safe failure when %s rejects and leaves the form retryable',
  async (action) => {
    const post = vi.spyOn(api, 'post').mockRejectedValue(
      new AxiosError(
        'Request failed',
        'ERR_BAD_RESPONSE',
        undefined,
        undefined,
        {
          data: { message: 'private backend diagnostic' },
          status: 500,
          statusText: 'Error',
          headers: {},
          config: {} as never,
        }
      )
    )
    const errors = vi.spyOn(toast, 'error')
    const router = createRouter({
      routeTree: createRootRoute({ component: SignUpForm }),
      history: createMemoryHistory({ initialEntries: ['/'] }),
    })
    if (action === 'send code') {
      client.setQueryData(STATUS_QUERY_KEY, { email_verification: true })
    }
    render(
      <QueryClientProvider client={client}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    )
    const user = userEvent.setup()
    await screen.findByLabelText('Username')
    if (action === 'register') {
      await user.type(screen.getByLabelText('Username'), 'test-user')
      await user.type(screen.getByLabelText('Password'), 'example-password')
      await user.type(
        screen.getByLabelText('Confirm password'),
        'example-password'
      )
      await user.click(screen.getByRole('button', { name: 'Create account' }))
    } else {
      await user.type(
        screen.getByLabelText('Email (required for verification)'),
        'test@example.com'
      )
      await user.click(screen.getByRole('button', { name: 'Send code' }))
    }
    await waitFor(() =>
      expect(errors).toHaveBeenCalledWith('Please try again later.')
    )
    expect(errors).toHaveBeenCalledTimes(1)
    expect(post).toHaveBeenCalledTimes(1)
    expect(
      screen.getByRole('button', {
        name: action === 'register' ? 'Create account' : 'Send code',
      })
    ).toBeEnabled()
  }
)

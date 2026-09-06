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
import { act, renderHook } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import type { SystemStatus } from '../../types'
import { useOAuthLogin } from '../use-oauth-login'

afterEach(() => vi.restoreAllMocks())

it('sends Telegram bot verification in the request body and resets it after failed authorization', async () => {
  const post = vi
    .spyOn(api, 'post')
    .mockRejectedValue(new Error('OAuth unavailable'))
  const reset = vi.fn()
  const { result } = renderHook(() =>
    useOAuthLogin(
      { telegram_oauth_configured: true } as SystemStatus,
      undefined,
      { turnstile: 'one-use-challenge', validate: () => true, reset }
    )
  )
  await act(async () => {
    await result.current.handleTelegramLogin()
  })
  expect(post).toHaveBeenCalledWith(
    '/api/oauth/state',
    expect.objectContaining({
      provider: 'telegram',
      intent: 'login',
      turnstile: 'one-use-challenge',
    }),
    expect.not.objectContaining({ params: expect.anything() })
  )
  expect(reset).toHaveBeenCalledOnce()
})

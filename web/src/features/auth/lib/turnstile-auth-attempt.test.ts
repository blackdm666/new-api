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
// @ts-expect-error The CI runtime provides bun:test; the application tsconfig intentionally omits Bun globals.
import { describe, expect, test } from 'bun:test'

import { runTurnstileProtectedAuthAttempt } from './turnstile-auth-attempt'

function createDeferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })

  return { promise, reject, resolve }
}

describe('runTurnstileProtectedAuthAttempt', () => {
  test('preserves the verified widget while a successful request is pending', async () => {
    const deferred = createDeferred<boolean>()
    let resetCount = 0
    const attempt = runTurnstileProtectedAuthAttempt(
      () => deferred.promise,
      () => {
        resetCount += 1
      }
    )

    expect(resetCount).toBe(0)
    deferred.resolve(true)

    expect(await attempt).toBe(true)
    expect(resetCount).toBe(0)
  })

  test('waits for an unsuccessful response before refreshing the widget', async () => {
    const deferred = createDeferred<boolean>()
    let resetCount = 0
    const attempt = runTurnstileProtectedAuthAttempt(
      () => deferred.promise,
      () => {
        resetCount += 1
      }
    )

    expect(resetCount).toBe(0)
    deferred.resolve(false)

    expect(await attempt).toBe(false)
    expect(resetCount).toBe(1)
  })

  test('refreshes the widget after a request error', async () => {
    const deferred = createDeferred<boolean>()
    let resetCount = 0
    const attempt = runTurnstileProtectedAuthAttempt(
      () => deferred.promise,
      () => {
        resetCount += 1
      }
    )

    deferred.reject(new Error('request failed'))

    await expect(attempt).rejects.toThrow('request failed')
    expect(resetCount).toBe(1)
  })
})

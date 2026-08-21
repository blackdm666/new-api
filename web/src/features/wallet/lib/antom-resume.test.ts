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
import { waitFor } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { watchAntomPaymentOnResume } from './antom-resume'

describe('Antom payment resume watcher', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  test('queries on focus and refreshes the balance after success', async () => {
    const syncPayment = vi.fn().mockResolvedValue('success')
    const refreshBalance = vi.fn().mockResolvedValue(undefined)
    const onFailure = vi.fn()
    const cleanup = watchAntomPaymentOnResume({
      tradeNo: 'ANTOM-FOCUS',
      syncPayment,
      onSuccess: refreshBalance,
      onFailure,
    })

    window.dispatchEvent(new Event('focus'))

    await waitFor(() => {
      expect(syncPayment).toHaveBeenCalledWith('ANTOM-FOCUS')
      expect(refreshBalance).toHaveBeenCalledTimes(1)
    })
    expect(onFailure).not.toHaveBeenCalled()
    cleanup()
  })
})

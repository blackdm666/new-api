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
import { afterEach, describe, expect, test, vi } from 'vitest'

import { queryAntomPayment, requestAntomPayment } from '../../api'
import { PAYMENT_TYPES } from '../../constants'
import { antomCheckoutNavigation, usePayment } from '../use-payment'

vi.mock('../../api', () => ({
  calculateAmount: vi.fn(),
  calculateStripeAmount: vi.fn(),
  calculateWaffoAmount: vi.fn(),
  calculateWaffoPancakeAmount: vi.fn(),
  requestPayment: vi.fn(),
  requestStripePayment: vi.fn(),
  requestAntomPayment: vi.fn(),
  queryAntomPayment: vi.fn(),
  isApiSuccess: (response: { success?: boolean; message?: string }) =>
    response.success === true || response.message === 'success',
}))

describe('Antom checkout flow', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.clearAllMocks()
    vi.unstubAllGlobals()
  })

  test('falls back to the current page when the popup is blocked', async () => {
    vi.spyOn(window, 'open').mockReturnValue(null)
    const assignCurrent = vi
      .spyOn(antomCheckoutNavigation, 'assignCurrent')
      .mockImplementation(() => undefined)
    vi.mocked(requestAntomPayment).mockResolvedValue({
      success: true,
      data: {
        normal_url: 'https://checkout.example/blocked',
        trade_no: 'ANTOM-BLOCKED',
      },
    })

    const { result } = renderHook(() => usePayment())
    await act(async () => {
      await result.current.processPayment(10, PAYMENT_TYPES.ANTOM)
    })

    expect(assignCurrent).toHaveBeenCalledWith(
      'https://checkout.example/blocked'
    )
  })

  test('uses current-page navigation for Safari', async () => {
    vi.stubGlobal('navigator', {
      userAgent:
        'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Version/17.5 Safari/605.1.15',
    })
    const open = vi.spyOn(window, 'open')
    const assignCurrent = vi
      .spyOn(antomCheckoutNavigation, 'assignCurrent')
      .mockImplementation(() => undefined)
    vi.mocked(requestAntomPayment).mockResolvedValue({
      success: true,
      data: {
        normal_url: 'https://checkout.example/safari',
        trade_no: 'ANTOM-SAFARI',
      },
    })

    const { result } = renderHook(() => usePayment())
    await act(async () => {
      await result.current.processPayment(10, PAYMENT_TYPES.ANTOM)
    })

    expect(open).not.toHaveBeenCalled()
    expect(assignCurrent).toHaveBeenCalledWith(
      'https://checkout.example/safari'
    )
  })

  test('uses current-page navigation if the loading tab closes early', async () => {
    let resolveRequest:
      | ((value: Awaited<ReturnType<typeof requestAntomPayment>>) => void)
      | undefined
    const popup = {
      closed: false,
      close: vi.fn(),
      document: { body: { textContent: '' }, title: '' },
      location: { replace: vi.fn() },
      opener: window,
    } as unknown as Window
    vi.spyOn(window, 'open').mockReturnValue(popup)
    const assignCurrent = vi
      .spyOn(antomCheckoutNavigation, 'assignCurrent')
      .mockImplementation(() => undefined)
    vi.mocked(requestAntomPayment).mockReturnValue(
      new Promise((resolve) => {
        resolveRequest = resolve
      })
    )

    const { result } = renderHook(() => usePayment())
    let processPromise: Promise<boolean> | undefined
    act(() => {
      processPromise = result.current.processPayment(10, PAYMENT_TYPES.ANTOM)
    })
    Object.defineProperty(popup, 'closed', { value: true })
    resolveRequest?.({
      success: true,
      data: {
        normal_url: 'https://checkout.example/closed',
        trade_no: 'ANTOM-CLOSED',
      },
    })
    await act(async () => {
      await processPromise
    })

    expect(assignCurrent).toHaveBeenCalledWith(
      'https://checkout.example/closed'
    )
  })

  test('opens a checkout tab before creating the session and records the pending trade', async () => {
    const replace = vi.fn()
    const close = vi.fn()
    const popup = {
      closed: false,
      close,
      document: { body: { textContent: '' }, title: '' },
      location: { replace },
      opener: window,
    } as unknown as Window
    vi.spyOn(window, 'open').mockReturnValue(popup)
    vi.mocked(requestAntomPayment).mockResolvedValue({
      success: true,
      data: {
        normal_url: 'https://checkout.example/session',
        trade_no: 'ANTOM-ORDER-1',
      },
    })

    const { result } = renderHook(() => usePayment())
    let success = false
    await act(async () => {
      success = await result.current.processPayment(10, PAYMENT_TYPES.ANTOM)
    })

    expect(success).toBe(true)
    expect(window.open).toHaveBeenCalledWith('', '_blank')
    expect(requestAntomPayment).toHaveBeenCalledWith({
      amount: 10,
      payment_method: PAYMENT_TYPES.ANTOM,
    })
    expect(replace).toHaveBeenCalledWith('https://checkout.example/session')
    expect(close).not.toHaveBeenCalled()
    expect(result.current.pendingAntomTradeNo).toBe('ANTOM-ORDER-1')
  })

  test('clears the pending trade after a successful inquiry', async () => {
    vi.mocked(queryAntomPayment).mockResolvedValue({
      success: true,
      data: {
        trade_no: 'ANTOM-ORDER-2',
        status: 'success',
        payment_method: 'ALIPAY_HK',
      },
    })

    const { result } = renderHook(() => usePayment())
    let status = null
    await act(async () => {
      status = await result.current.syncAntomPayment('ANTOM-ORDER-2')
    })

    expect(status).toBe('success')
    expect(queryAntomPayment).toHaveBeenCalledWith('ANTOM-ORDER-2')
    expect(result.current.pendingAntomTradeNo).toBeNull()
  })
})

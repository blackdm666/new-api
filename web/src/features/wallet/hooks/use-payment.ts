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
import i18next from 'i18next'
import { useState, useCallback } from 'react'
import { toast } from 'sonner'

import {
  calculateAmount,
  calculateStripeAmount,
  calculateWaffoAmount,
  calculateWaffoPancakeAmount,
  requestPayment,
  requestStripePayment,
  requestAntomPayment,
  queryAntomPayment,
  isApiSuccess,
} from '../api'
import {
  isStripePayment,
  isWaffoPayment,
  isWaffoPancakePayment,
  isAntomPayment,
  submitPaymentForm,
} from '../lib'
import type { AmountRequest, AmountResponse, TopupStatus } from '../types'

export const antomCheckoutNavigation = {
  assignCurrent(url: string) {
    window.location.assign(url)
  },
}

export function shouldUseCurrentTabForAntom(userAgent: string) {
  return (
    /Safari/i.test(userAgent) &&
    !/(Chrome|Chromium|CriOS|FxiOS|Edg|OPiOS|Android)/i.test(userAgent)
  )
}

// ============================================================================
// Payment Hook
// ============================================================================

type AmountCalculator = (request: AmountRequest) => Promise<AmountResponse>

export interface PaymentAmountCalculators {
  regular: AmountCalculator
  stripe: AmountCalculator
  waffo: AmountCalculator
  waffoPancake: AmountCalculator
}

const defaultPaymentAmountCalculators: PaymentAmountCalculators = {
  regular: calculateAmount,
  stripe: calculateStripeAmount,
  waffo: calculateWaffoAmount,
  waffoPancake: calculateWaffoPancakeAmount,
}

export async function requestPaymentAmount(
  topupAmount: number,
  paymentType: string,
  calculators: PaymentAmountCalculators = defaultPaymentAmountCalculators
): Promise<number> {
  let calculator = calculators.regular
  if (isStripePayment(paymentType)) {
    calculator = calculators.stripe
  } else if (isWaffoPayment(paymentType)) {
    calculator = calculators.waffo
  } else if (isWaffoPancakePayment(paymentType)) {
    calculator = calculators.waffoPancake
  }

  const response = await calculator({ amount: topupAmount })
  if (!isApiSuccess(response) || !response.data) {
    return 0
  }

  return Number.parseFloat(response.data)
}

export function usePayment() {
  const [amount, setAmount] = useState<number>(0)
  const [calculating, setCalculating] = useState(false)
  const [processing, setProcessing] = useState(false)
  const [pendingAntomTradeNo, setPendingAntomTradeNo] = useState<string | null>(
    null
  )

  // Calculate payment amount
  const calculatePaymentAmount = useCallback(
    async (topupAmount: number, paymentType: string) => {
      try {
        setCalculating(true)
        const calculatedAmount = await requestPaymentAmount(
          topupAmount,
          paymentType
        )
        setAmount(calculatedAmount)
        return calculatedAmount
      } catch {
        setAmount(0)
        return 0
      } finally {
        setCalculating(false)
      }
    },
    []
  )

  // Process payment
  const processPayment = useCallback(
    async (topupAmount: number, paymentType: string) => {
      let checkoutWindow: Window | null = null
      try {
        setProcessing(true)

        const isStripe = isStripePayment(paymentType)
        const isAntom = isAntomPayment(paymentType)
        const amount = Math.floor(topupAmount)

        if (isAntom) {
          if (!shouldUseCurrentTabForAntom(window.navigator.userAgent)) {
            checkoutWindow = window.open('', '_blank')
          }
          if (checkoutWindow) {
            checkoutWindow.document.title = i18next.t(
              'Preparing secure checkout...'
            )
            if (checkoutWindow.document.body) {
              checkoutWindow.document.body.textContent = i18next.t(
                'Preparing secure checkout...'
              )
            }
          }
          const response = await requestAntomPayment({
            amount,
            payment_method: paymentType,
          })
          if (
            !isApiSuccess(response) ||
            !response.data?.normal_url ||
            !response.data.trade_no
          ) {
            checkoutWindow?.close()
            toast.error(response.message || i18next.t('Payment request failed'))
            return false
          }
          setPendingAntomTradeNo(response.data.trade_no)
          if (checkoutWindow && !checkoutWindow.closed) {
            checkoutWindow.opener = null
            checkoutWindow.location.replace(response.data.normal_url)
          } else {
            antomCheckoutNavigation.assignCurrent(response.data.normal_url)
          }
          toast.success(i18next.t('Redirecting to payment page...'))
          return true
        }

        const response = isStripe
          ? await requestStripePayment({
              amount,
              payment_method: 'stripe',
            })
          : await requestPayment({
              amount,
              payment_method: paymentType,
            })

        if (!isApiSuccess(response)) {
          toast.error(response.message || i18next.t('Payment request failed'))
          return false
        }

        if (isStripe && response.data?.pay_link) {
          window.open(response.data.pay_link as string, '_blank')
          toast.success(i18next.t('Redirecting to payment page...'))
          return true
        }

        if (!isStripe && response.data) {
          const url = (response as unknown as { url?: string }).url
          if (url) {
            submitPaymentForm(url, response.data)
            toast.success(i18next.t('Redirecting to payment page...'))
            return true
          }
        }

        return false
      } catch {
        checkoutWindow?.close()
        toast.error(i18next.t('Payment request failed'))
        return false
      } finally {
        setProcessing(false)
      }
    },
    []
  )

  const syncAntomPayment = useCallback(
    async (tradeNo?: string): Promise<TopupStatus | null> => {
      const targetTradeNo = tradeNo || pendingAntomTradeNo
      if (!targetTradeNo) return null

      try {
        const response = await queryAntomPayment(targetTradeNo)
        if (!isApiSuccess(response) || !response.data) {
          return null
        }
        const status = response.data.status
        if (status !== 'pending') {
          setPendingAntomTradeNo(null)
        }
        return status
      } catch {
        return null
      }
    },
    [pendingAntomTradeNo]
  )

  return {
    amount,
    calculating,
    processing,
    calculatePaymentAmount,
    processPayment,
    pendingAntomTradeNo,
    syncAntomPayment,
    setAmount,
  }
}

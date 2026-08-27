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
  convertBillingCurrencyToUSD,
  convertUSDToBillingCurrency,
  type BillingCurrencyConversion,
} from '@/lib/currency'

import { formatPricingNumber } from './pricing-format'

export function pricingUSDToDisplay(
  value: unknown,
  currency: BillingCurrencyConversion
): string {
  if (value === '' || value === null || value === undefined) return ''
  const numeric = Number(value)
  if (!Number.isFinite(numeric)) return ''
  return formatPricingNumber(convertUSDToBillingCurrency(numeric, currency))
}

export function pricingDisplayToUSD(
  value: unknown,
  currency: BillingCurrencyConversion
): string {
  if (value === '' || value === null || value === undefined) return ''
  const numeric = Number(value)
  if (!Number.isFinite(numeric)) return ''
  return formatPricingNumber(convertBillingCurrencyToUSD(numeric, currency))
}

export function pricingDisplayToModelRatio(
  value: unknown,
  currency: BillingCurrencyConversion
): string {
  const usd = pricingDisplayToUSD(value, currency)
  if (usd === '') return ''
  return formatPricingNumber(Number(usd) / 2)
}

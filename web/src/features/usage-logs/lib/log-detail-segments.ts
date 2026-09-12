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
import { formatBillingCurrencyFromUSD } from '@/lib/currency'
import { formatLogQuota } from '@/lib/format'

import type { UsageLog } from '../data/schema'
import type { LogOtherData } from '../types'
import {
  applyLoggedGroupRatio,
  formatRatioCompact,
  getEffectiveGroupRatioInfo,
} from './billing-display'
import {
  isLegacyTaskFixedBilling,
  isPerCallBilling,
  isPerSecondBilling,
} from './billing-unit'
import {
  getTieredBillingSummary,
  hasAnyCacheTokens,
  isViolationFeeLog,
  renderAuditContent,
} from './format'

export interface DetailSegment {
  text: string
  muted?: boolean
  danger?: boolean
}

export function buildTypeDetailSegments(
  log: UsageLog,
  other: LogOtherData | null,
  t: (key: string, opts?: Record<string, unknown>) => string
): DetailSegment[] {
  // Audit (type=3) and login (type=7) logs: render localized content from the
  // structured op descriptor instead of the raw (English-fallback) content.
  if (log.type === 3 || log.type === 7) {
    const text = renderAuditContent(other, t)
    return text ? [{ text }] : []
  }

  if (log.type === 6) {
    return [{ text: t('Async task refund') }]
  }

  if (log.type !== 2) return []

  const isViolation = isViolationFeeLog(other)
  if (isViolation) {
    const segments: DetailSegment[] = []
    segments.push({ text: t('Violation Fee'), danger: true })
    if (other?.violation_fee_code) {
      segments.push({
        text: other.violation_fee_code,
        muted: true,
      })
    }
    segments.push({
      text: `${t('Fee')}: ${formatLogQuota(other?.fee_quota ?? log.quota)}`,
      muted: true,
    })
    return segments
  }

  if (!other) return []

  const segments: DetailSegment[] = []
  const priceOpts = { digitsLarge: 4, digitsSmall: 6, abbreviate: false }
  const actualPrice = (basePrice: number) =>
    applyLoggedGroupRatio(basePrice, other)
  const formatPrice = (price: number) =>
    `${formatBillingCurrencyFromUSD(price, priceOpts)}/M`
  const formatPriceCompact = (price: number) =>
    formatBillingCurrencyFromUSD(price, priceOpts)
  const formatPriceList = (prices: string[], showUnit: boolean) => {
    const text = prices.join(' / ')
    return showUnit ? `${text}/M` : text
  }
  const isTieredExpr = other.billing_mode === 'tiered_expr'
  const tieredSummary = getTieredBillingSummary(other)
  if (isTieredExpr) {
    if (tieredSummary) {
      const baseEntries = tieredSummary.priceEntries
        .filter((entry) => ['inputPrice', 'outputPrice'].includes(entry.field))
        .map((entry) => formatPriceCompact(actualPrice(entry.price)))
      if (baseEntries.length > 0) {
        const tierLabel = tieredSummary.tier.label || t('Default')
        segments.push({
          text: `${tierLabel} · ${formatPriceList(baseEntries, true)}`,
        })
      }

      const cacheEntries = tieredSummary.priceEntries
        .filter((entry) =>
          ['cacheReadPrice', 'cacheCreatePrice', 'cacheCreate1hPrice'].includes(
            entry.field
          )
        )
        .map((entry) => formatPriceCompact(actualPrice(entry.price)))
      if (cacheEntries.length > 0) {
        segments.push({
          text: `${t('Cache')} ${formatPriceList(cacheEntries, false)}`,
          muted: true,
        })
      }

      const otherEntries = tieredSummary.priceEntries
        .filter(
          (entry) =>
            ![
              'inputPrice',
              'outputPrice',
              'cacheReadPrice',
              'cacheCreatePrice',
              'cacheCreate1hPrice',
            ].includes(entry.field)
        )
        .map(
          (entry) =>
            `${t(entry.shortLabel)} ${formatPrice(actualPrice(entry.price))}`
        )
      if (otherEntries.length > 0) {
        segments.push({
          text: otherEntries.join(' · '),
          muted: true,
        })
      }
    } else {
      segments.push({
        text: `${t('Dynamic Pricing')} · ${t('No matching results')}`,
        muted: true,
      })
    }
  } else {
    const modelPrice = other.model_price
    const isPerSecond = isPerSecondBilling(other.billing_unit)
    const isTask = other.is_task === true
    const isPerCall = isPerCallBilling(modelPrice, other.billing_unit, isTask)
    const isLegacyTaskFixed = isLegacyTaskFixedBilling(
      modelPrice,
      other.billing_unit,
      isTask
    )
    if (isPerSecond && modelPrice != null) {
      segments.push({
        text: `${t('Per-second')} · ${formatBillingCurrencyFromUSD(actualPrice(modelPrice), priceOpts)}/${t('second')}`,
      })
    } else if (isPerCall && modelPrice != null) {
      segments.push({
        text: `${t('Per-call')} · ${formatBillingCurrencyFromUSD(actualPrice(modelPrice), priceOpts)}`,
      })
    } else if (isLegacyTaskFixed && modelPrice != null) {
      segments.push({
        text: `${t('Dynamic Pricing')} · ${formatBillingCurrencyFromUSD(actualPrice(modelPrice), priceOpts)}`,
      })
    } else if (other.model_ratio != null) {
      const inputPriceUSD = other.model_ratio * 2.0
      const baseEntries = [formatPriceCompact(actualPrice(inputPriceUSD))]
      if (other.completion_ratio != null) {
        baseEntries.push(
          formatPriceCompact(
            actualPrice(inputPriceUSD * other.completion_ratio)
          )
        )
      }
      segments.push({
        text: `${t('Standard')} · ${formatPriceList(baseEntries, true)}`,
      })

      if (hasAnyCacheTokens(other)) {
        const cacheEntries = [
          other.cache_ratio != null && other.cache_ratio !== 1
            ? formatPriceCompact(actualPrice(inputPriceUSD * other.cache_ratio))
            : null,
          other.cache_creation_ratio != null && other.cache_creation_ratio !== 1
            ? formatPriceCompact(
                actualPrice(inputPriceUSD * other.cache_creation_ratio)
              )
            : null,
          other.cache_creation_ratio_1h != null &&
          other.cache_creation_ratio_1h !== 0
            ? formatPriceCompact(
                actualPrice(inputPriceUSD * other.cache_creation_ratio_1h)
              )
            : null,
        ].filter(Boolean) as string[]

        if (cacheEntries.length > 0) {
          segments.push({
            text: `${t('Cache')} ${formatPriceList(cacheEntries, false)}`,
            muted: true,
          })
        }
      }
    } else {
      const ratioInfo = getEffectiveGroupRatioInfo(other)
      if (ratioInfo) {
        const ratioLabel = ratioInfo.isUserSpecific
          ? t('User Exclusive Ratio')
          : t('Group Ratio')
        segments.push({
          text: `${ratioLabel} ${formatRatioCompact(ratioInfo.ratio)}x`,
        })
      }
    }
  }

  if (other.is_system_prompt_overwritten) {
    segments.push({
      text: t('System Prompt Override'),
      danger: true,
    })
  }

  return segments
}

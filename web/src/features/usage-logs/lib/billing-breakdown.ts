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
import type { TFunction } from 'i18next'

import { formatBillingCurrencyFromUSD } from '@/lib/currency'
import { formatLogQuota } from '@/lib/format'

import type { UsageLog } from '../data/schema'
import { USAGE_BILLING_PATH, type LogOtherData } from '../types'
import {
  applyLoggedGroupRatio,
  formatRatioCompact,
  getEffectiveGroupRatioInfo,
} from './billing-display'
import { getTieredBillingSummary, hasAnyCacheTokens } from './format'
import { isPerCallBilling } from './utils'

export interface BillingBreakdownRow {
  label: string
  value: string
}

export function getUsageBillingPathLabel(
  t: TFunction,
  adminInfo: LogOtherData['admin_info']
): string {
  switch (adminInfo?.usage_billing_path) {
    case USAGE_BILLING_PATH.LOCAL:
      return t('Local Billing')
    case USAGE_BILLING_PATH.OPENAI:
      return t('Upstream Response (billing-usage-openai)')
    case USAGE_BILLING_PATH.OPENAI_ESTIMATED:
      return t('Upstream Response (billing-usage-openai-estimated)')
    case USAGE_BILLING_PATH.ANTHROPIC:
      return t('Upstream Response (billing-usage-anthropic)')
    case USAGE_BILLING_PATH.ANTHROPIC_ESTIMATED:
      return t('Upstream Response (billing-usage-anthropic-estimated)')
    case USAGE_BILLING_PATH.GEMINI:
      return t('Upstream Response (billing-usage-gemini)')
    case USAGE_BILLING_PATH.GEMINI_ESTIMATED:
      return t('Upstream Response (billing-usage-gemini-estimated)')
    case USAGE_BILLING_PATH.UPSTREAM:
      return t('Upstream Response')
    default:
      return adminInfo?.local_count_tokens
        ? t('Local Billing')
        : t('Upstream Response')
  }
}

export function buildBillingBreakdownRows(
  log: UsageLog,
  other: LogOtherData,
  isAdmin: boolean,
  t: TFunction
): BillingBreakdownRow[] {
  const isPerCall = isPerCallBilling(other.model_price)
  const isClaude = other.claude === true
  const isTieredExpr = other.billing_mode === 'tiered_expr'
  const tieredSummary = getTieredBillingSummary(other)

  const rows: BillingBreakdownRow[] = []
  const priceOpts = { digitsLarge: 4, digitsSmall: 6, abbreviate: false }
  const fmtPrice = (usd: number) => formatBillingCurrencyFromUSD(usd, priceOpts)
  const actualPrice = (basePrice: number) =>
    applyLoggedGroupRatio(basePrice, other)
  const baseInputUSD = other.model_ratio != null ? other.model_ratio * 2.0 : 0
  const groupRatioInfo = getEffectiveGroupRatioInfo(other)
  const pushPriceExplanation = () => {
    if (!groupRatioInfo) return
    rows.push({
      label: t('Price Explanation'),
      value: t('Adjusted by {{ratio}}x group rate', {
        ratio: formatRatioCompact(groupRatioInfo.ratio),
      }),
    })
  }

  if (isTieredExpr) {
    rows.push({
      label: t('Billing Mode'),
      value: t('Dynamic Pricing'),
    })
    pushPriceExplanation()
    if (tieredSummary) {
      if (tieredSummary.tier.label) {
        rows.push({
          label: t('Matched Tier'),
          value: tieredSummary.tier.label,
        })
      }
      for (const entry of tieredSummary.priceEntries) {
        rows.push({
          label: t(entry.shortLabel),
          value: `${fmtPrice(actualPrice(entry.price))}/M`,
        })
      }
    } else {
      rows.push({
        label: t('Matched Tier'),
        value: t('No matching results'),
      })
    }
  } else if (isPerCall) {
    rows.push({ label: t('Billing Mode'), value: t('Per-call') })
    pushPriceExplanation()
    if (other.model_price != null) {
      rows.push({
        label: t('Model Price'),
        value: fmtPrice(actualPrice(other.model_price)),
      })
    }
  } else {
    rows.push({ label: t('Billing Mode'), value: t('Per-token') })
    pushPriceExplanation()
    if (other.model_ratio != null) {
      rows.push({
        label: t('Input'),
        value: `${fmtPrice(actualPrice(baseInputUSD))}/M`,
      })
    }
    if (other.completion_ratio != null && other.model_ratio != null) {
      rows.push({
        label: t('Output'),
        value: `${fmtPrice(
          actualPrice(baseInputUSD * other.completion_ratio)
        )}/M`,
      })
    }
  }

  if (!isTieredExpr && isClaude && hasAnyCacheTokens(other)) {
    if (other.cache_ratio != null && other.cache_ratio !== 1) {
      rows.push({
        label: t('Cache Read'),
        value: `${fmtPrice(actualPrice(baseInputUSD * other.cache_ratio))}/M`,
      })
    }
    if (
      other.cache_creation_ratio != null &&
      other.cache_creation_ratio !== 1
    ) {
      rows.push({
        label: t('Cache Creation'),
        value: `${fmtPrice(
          actualPrice(baseInputUSD * other.cache_creation_ratio)
        )}/M`,
      })
    }
    if (
      other.cache_creation_ratio_5m != null &&
      other.cache_creation_ratio_5m !== 0
    ) {
      rows.push({
        label: t('Cache Creation (5m)'),
        value: `${fmtPrice(
          actualPrice(baseInputUSD * other.cache_creation_ratio_5m)
        )}/M`,
      })
    }
    if (
      other.cache_creation_ratio_1h != null &&
      other.cache_creation_ratio_1h !== 0
    ) {
      rows.push({
        label: t('Cache Creation (1h)'),
        value: `${fmtPrice(
          actualPrice(baseInputUSD * other.cache_creation_ratio_1h)
        )}/M`,
      })
    }
  }

  if (!isTieredExpr) {
    if (other.audio_ratio != null && other.audio_ratio !== 1) {
      rows.push({
        label: t('Audio input'),
        value: `${fmtPrice(actualPrice(baseInputUSD * other.audio_ratio))}/M`,
      })
    }

    if (
      other.audio_completion_ratio != null &&
      other.audio_completion_ratio !== 1
    ) {
      rows.push({
        label: t('Audio output'),
        value: `${fmtPrice(
          actualPrice(baseInputUSD * other.audio_completion_ratio)
        )}/M`,
      })
    }

    if (other.image_ratio != null && other.image_ratio !== 1) {
      rows.push({
        label: t('Image input'),
        value: `${fmtPrice(actualPrice(baseInputUSD * other.image_ratio))}/M`,
      })
    }
  }

  if (other.web_search && other.web_search_call_count) {
    rows.push({
      label: t('Web Search'),
      value: `${other.web_search_call_count}x${other.web_search_price ? ` (${fmtPrice(actualPrice(other.web_search_price))})` : ''}`,
    })
  }

  if (other.file_search && other.file_search_call_count) {
    rows.push({
      label: t('File Search'),
      value: `${other.file_search_call_count}x${other.file_search_price ? ` (${fmtPrice(actualPrice(other.file_search_price))})` : ''}`,
    })
  }

  if (other.image_generation_call && other.image_generation_call_price) {
    rows.push({
      label: t('Image Generation'),
      value: fmtPrice(actualPrice(other.image_generation_call_price)),
    })
  }

  if (other.audio_input_seperate_price && other.audio_input_price) {
    rows.push({
      label: t('Audio Input Price'),
      value: fmtPrice(actualPrice(other.audio_input_price)),
    })
  }

  if (isAdmin && other.admin_info) {
    rows.push({
      label: t('Billing Path'),
      value: getUsageBillingPathLabel(t, other.admin_info),
    })
  }

  rows.push({
    label: t('Total Cost'),
    value: formatLogQuota(log.quota),
  })

  return rows
}

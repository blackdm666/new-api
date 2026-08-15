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
import { formatLocalCurrencyAmount } from '@/lib/currency'

import {
  AFFILIATE_COMMISSION_STATUS,
  AFFILIATE_PAYOUT_STATUS,
  type AffiliateCommissionStatus,
  type AffiliatePayoutStatus,
} from './types'

export const AFFILIATE_STATUS_META: Record<
  AffiliateCommissionStatus,
  { labelKey: string; className: string }
> = {
  [AFFILIATE_COMMISSION_STATUS.PENDING]: {
    labelKey: 'Pending review',
    className: 'border-amber-500/40 bg-amber-500/10 text-amber-600',
  },
  [AFFILIATE_COMMISSION_STATUS.APPROVED]: {
    labelKey: 'Approved',
    className: 'border-emerald-500/40 bg-emerald-500/10 text-emerald-600',
  },
  [AFFILIATE_COMMISSION_STATUS.REJECTED]: {
    labelKey: 'Rejected',
    className: 'border-destructive/40 bg-destructive/10 text-destructive',
  },
}

export const AFFILIATE_PAYOUT_STATUS_META: Record<
  AffiliatePayoutStatus,
  { labelKey: string; className: string }
> = {
  [AFFILIATE_PAYOUT_STATUS.PENDING]: {
    labelKey: 'Pending review',
    className: 'border-amber-500/40 bg-amber-500/10 text-amber-600',
  },
  [AFFILIATE_PAYOUT_STATUS.APPROVED]: {
    labelKey: 'Approved for payout',
    className: 'border-blue-500/40 bg-blue-500/10 text-blue-600',
  },
  [AFFILIATE_PAYOUT_STATUS.PAID]: {
    labelKey: 'Paid',
    className: 'border-emerald-500/40 bg-emerald-500/10 text-emerald-600',
  },
  [AFFILIATE_PAYOUT_STATUS.REJECTED]: {
    labelKey: 'Rejected',
    className: 'border-destructive/40 bg-destructive/10 text-destructive',
  },
  [AFFILIATE_PAYOUT_STATUS.CANCELLED]: {
    labelKey: 'Cancelled',
    className: 'border-muted-foreground/30 bg-muted text-muted-foreground',
  },
  [AFFILIATE_PAYOUT_STATUS.PROCESSING]: {
    labelKey: 'Paying via Alipay',
    className: 'border-violet-500/40 bg-violet-500/10 text-violet-600',
  },
}

export function formatCents(cents: number | null | undefined): string {
  return formatLocalCurrencyAmount((cents ?? 0) / 100, {
    digitsLarge: 2,
    digitsSmall: 2,
  })
}

export function formatRate(rateBasisPoints: number): string {
  return `${(rateBasisPoints / 100)
    .toFixed(2)
    .replace(/\.00$/, '')
    .replace(/(\.\d)0$/, '$1')}%`
}

export function promoterTierLabelKey(tier: string | null | undefined): string {
  switch (tier?.trim()) {
    case 'default':
    case '默认推广':
    case '初级推广':
      return 'Junior promoter'
    case '高级推广':
      return 'Advanced promoter'
    case '金牌推广':
      return 'Gold promoter'
    default:
      return 'Default promoter'
  }
}

export function promoterTierBadgeClassName(
  tier: string | null | undefined
): string {
  switch (tier?.trim()) {
    case '高级推广':
      return 'border-violet-500/40 bg-violet-500/10 text-violet-700 dark:text-violet-300'
    case '金牌推广':
      return 'border-amber-500/45 bg-amber-500/12 text-amber-700 dark:text-amber-300'
    case 'default':
    case '默认推广':
    case '初级推广':
    default:
      return 'border-sky-500/40 bg-sky-500/10 text-sky-700 dark:text-sky-300'
  }
}

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

import {
  INVOICE_STATUS,
  type InvoiceRequest,
  type InvoiceStatus,
} from '../types'

/** Unified social credit codes are 18 characters; legacy tax numbers vary. */
export const INVOICE_TAX_NUMBER_LENGTH = { min: 15, max: 20 } as const

export function normalizeInvoiceTaxNumber(value: string): string {
  return value.trim().toUpperCase()
}

// Invoice amounts are always CNY, so they use a fixed grouping format that
// matches what is typed into the invoicing system, e.g. ¥2,979.00.
const invoiceAmountFormat = new Intl.NumberFormat('en-US', {
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
})

export function formatInvoiceAmount(yuan: number): string {
  return `¥${invoiceAmountFormat.format(Number(yuan) || 0)}`
}

export function getInvoiceAmountShortfall(
  amount: number,
  minimumAmount: number
): number {
  return Math.max(0, minimumAmount - amount)
}

export function calculateInvoiceTaxFee(
  amount: number,
  rateBasisPoints: number
): number {
  return Math.round((amount * 100 * rateBasisPoints) / 10000) / 100
}

export const INVOICE_STATUS_META: Record<
  InvoiceStatus,
  { labelKey: string; badgeClass: string }
> = {
  [INVOICE_STATUS.PENDING]: {
    labelKey: 'Pending processing',
    badgeClass:
      'border-amber-500/40 bg-amber-500/10 text-amber-600 dark:text-amber-400',
  },
  [INVOICE_STATUS.ISSUED]: {
    labelKey: 'Issued',
    badgeClass:
      'border-emerald-500/40 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400',
  },
  [INVOICE_STATUS.REJECTED]: {
    labelKey: 'Rejected',
    badgeClass:
      'border-rose-500/40 bg-rose-500/10 text-rose-600 dark:text-rose-400',
  },
  [INVOICE_STATUS.WITHDRAWN]: {
    labelKey: 'Withdrawn',
    badgeClass:
      'border-slate-500/40 bg-slate-500/10 text-slate-600 dark:text-slate-300',
  },
  [INVOICE_STATUS.EXPIRED]: {
    labelKey: 'Expired',
    badgeClass:
      'border-orange-500/40 bg-orange-500/10 text-orange-600 dark:text-orange-400',
  },
}

export function formatInvoiceTimestamp(
  unixSec?: number,
  locale?: string
): string {
  if (!unixSec) return '—'
  return new Date(unixSec * 1000).toLocaleString(locale ?? [], {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
}

export function isPreviewableInvoiceFile(mime: string): boolean {
  const normalized = mime.toLowerCase().split(';', 1)[0].trim()
  return normalized.startsWith('image/') && normalized !== 'image/svg+xml'
}

export function isInvoiceRequestExpiring(invoice: InvoiceRequest): boolean {
  if (invoice.status !== INVOICE_STATUS.PENDING) return false
  if (Number(invoice.expiry_warning_time || 0) > 0) return true
  const expiresAt = Number(invoice.expires_at || 0)
  if (!expiresAt) return false
  return expiresAt * 1000 <= Date.now() + 24 * 60 * 60 * 1000
}

export function getInvoiceNotificationKindLabel(
  kind: string,
  t: TFunction
): string {
  if (kind === 'admin_email') return t('Admin email')
  if (kind === 'user_email' || kind === 'user') return t('User email')
  // Rejection and expiry notices to the applicant (no attachments).
  if (kind === 'user_notice') return t('User notice email')
  if (kind === 'wechat_group') return t('Finance WeChat group')
  return kind
}

export function getInvoiceNotificationChannelLabel(
  channel: string | undefined,
  t: TFunction
): string {
  // Deliveries recorded before WeChat support carry no channel and are email.
  return channel === 'wechat' ? t('WeChat group') : t('Email channel')
}

export function getInvoiceStorageTypeLabel(
  storageType: string,
  t: TFunction
): string {
  if (storageType === 'local') return t('Local disk')
  if (storageType === 'oss') return t('Alibaba Cloud OSS')
  if (storageType === 's3') return t('S3 compatible storage')
  if (storageType === 'cos') return t('Tencent Cloud COS')
  return storageType
}

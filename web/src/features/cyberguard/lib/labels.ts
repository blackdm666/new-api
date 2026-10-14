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

import type { StatusVariant } from '@/components/status-badge'
import { formatTimestampToDate } from '@/lib/format'

// Unknown values from a newer backend fall back to the raw identifier.

export function modeLabel(mode: string, t: TFunction): string {
  if (mode === 'off') return t('Off')
  if (mode === 'observe') return t('Observe only')
  if (mode === 'enforce') return t('Auto ban')
  return mode
}

export const MODE_VARIANT: Record<string, StatusVariant> = {
  off: 'neutral',
  observe: 'warning',
  enforce: 'danger',
}

export function categoryLabel(category: string, t: TFunction): string {
  if (category === 'cyber') return t('Cybersecurity')
  if (category === 'bio') return t('Biological risk')
  if (category === 'usage') return t('Usage policy')
  if (category === 'session_blocked') return t('Session blocked')
  return category
}

export const CATEGORY_VARIANT: Record<string, StatusVariant> = {
  cyber: 'danger',
  bio: 'purple',
  usage: 'info',
  session_blocked: 'neutral',
}

export function eventStateLabel(state: string, t: TFunction): string {
  switch (state) {
    case 'pending':
      return t('Awaiting attribution')
    case 'counted':
      return t('Counted')
    case 'record_only':
      return t('Record only')
    case 'unattributed':
      return t('Unattributed')
    case 'exempt':
      return t('Exempt')
    case 'void':
      return t('Voided')
    default:
      return state
  }
}

export const EVENT_STATE_VARIANT: Record<string, StatusVariant> = {
  pending: 'warning',
  counted: 'danger',
  record_only: 'neutral',
  unattributed: 'neutral',
  exempt: 'info',
  void: 'neutral',
}

export function banStatusLabel(status: string, t: TFunction): string {
  if (status === 'observed') return t('Proposed')
  if (status === 'active') return t('Banned')
  if (status === 'ended') return t('Ended')
  if (status === 'dismissed') return t('Dismissed')
  return status
}

export const BAN_STATUS_VARIANT: Record<string, StatusVariant> = {
  observed: 'warning',
  active: 'danger',
  ended: 'neutral',
  dismissed: 'neutral',
}

export function banSourceLabel(source: string, t: TFunction): string {
  if (source === 'auto') return t('Automatic')
  if (source === 'manual') return t('Manual')
  if (source === 'adopted') return t('Imported')
  return source
}

export function endReasonLabel(reason: string, t: TFunction): string {
  switch (reason) {
    case 'expired':
      return t('Expired and restored')
    case 'manual_unban':
      return t('Unbanned manually')
    case 'admin_override':
      return t('Group changed by an administrator')
    case 'promoted':
      return t('Turned into a ban')
    case 'dismissed':
      return t('Dismissed')
    case 'user_missing':
      return t('User no longer exists')
    default:
      return reason
  }
}

/** Ban-row email state, refined by the live outbox state when one exists. */
export function emailStateLabel(
  state: string,
  deliveryState: string,
  t: TFunction
): string {
  switch (deliveryState) {
    case 'queued':
      return t('Queued')
    case 'sending':
      return t('Sending')
    case 'retrying':
      return t('Waiting for retry')
    case 'awaiting_receipt':
      return t('Waiting for receipt')
    case 'accepted_untracked':
      return t('SMTP accepted, untracked')
    case 'delivered':
      return t('Final delivery confirmed')
    case 'failed':
      return t('Failed')
    case 'expired':
      return t('Expired')
    default:
      break
  }
  switch (state) {
    case 'none':
      return t('No email')
    case 'disabled':
      return t('Email disabled')
    case 'queued':
      return t('Queued')
    case 'skipped_no_email':
      return t('No email address')
    case 'failed':
      return t('Failed')
    default:
      return state || '—'
  }
}

/** Planned duration of a ban; 0 hours means until a manual unban. */
export function banDurationLabel(hours: number, t: TFunction): string {
  if (hours <= 0) return t('Until manual unban')
  return t('{{hours}} hours', { hours })
}

export function banExpiryLabel(expiresAt: number, t: TFunction): string {
  if (expiresAt <= 0) return t('Until manual unban')
  return formatTimestampToDate(expiresAt)
}

/** sub2api account status; "unknown" when sub2api could not be asked. */
export function accountStatusLabel(status: string, t: TFunction): string {
  if (status === 'active') return t('Enabled')
  if (status === 'disabled') return t('Disabled')
  if (status === 'error') return t('Error')
  if (status === 'unknown') return t('Unknown')
  return status
}

/** sub2api account schedulability; null when sub2api could not be asked. */
export function schedulableLabel(value: boolean | null, t: TFunction): string {
  if (value === null) return t('Unknown')
  return value ? t('Yes') : t('No')
}

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
export const AFFILIATE_COMMISSION_STATUS = {
  PENDING: 1,
  APPROVED: 2,
  REJECTED: 3,
} as const

export type AffiliateCommissionStatus =
  (typeof AFFILIATE_COMMISSION_STATUS)[keyof typeof AFFILIATE_COMMISSION_STATUS]

export type ApiResponse<T = unknown> = {
  success?: boolean
  message?: string
  data?: T
}

export type PaginatedResponse<T> = {
  items: T[]
  total: number
}

export type AffiliateSummary = {
  auto_approve: boolean
  available_quota: number
  available_cents: number
  total_approved_quota: number
  pending_commission_cents: number
  approved_commission_cents: number
  total_topup_cents: number
  invite_count: number
  effective_invitee_count: number
  commission_record_count: number
  rate_basis_points: number
  default_rate_basis_points: number
  group_rates: Record<string, number>
  tier_name: string
  upgrade_eligible: boolean
  next_tier_name: string
  next_tier_rate_basis_points: number
  upgrade_threshold: number
  upgrade_progress: number
  upgrade_progress_ratio: number
  upgrade_top_up_amount_threshold_cents: number
  upgrade_top_up_amount_progress_cents: number
  upgrade_top_up_amount_progress_ratio: number
}

export type AffiliateCommission = {
  id: number
  inviter_id: number
  invitee_id: number
  topup_id: number
  trade_no: string
  topup_amount_cents: number
  rate_basis_points: number
  inviter_group: string
  tier_name: string
  commission_cents: number
  commission_quota: number
  status: AffiliateCommissionStatus
  reject_reason?: string
  operator_id?: number
  approved_time?: number
  created_time: number
  updated_time: number
  inviter_username?: string
  inviter_display_name?: string
  invitee_username?: string
  invitee_display_name?: string
}

export type AffiliateInviteeStats = {
  username: string
  display_name: string
  created_at: number
  is_new: boolean
  topup_count: number
  topup_amount_cents: number
  commission_cents: number
  last_topup_time: number
}

export type AffiliateAdminSummary = {
  pending_count: number
  pending_cents: number
  approved_cents: number
  effective_invitee_count: number
  topup_cents: number
  commission_record_count: number
}

export type AffiliateTransfer = {
  id: number
  user_id: number
  request_id: string
  amount_cents: number
  amount_quota: number
  balance_cents_before: number
  balance_cents_after: number
  quota_before: number
  quota_after: number
  created_time: number
  username?: string
  display_name?: string
}

export type AffiliateUpgradeCandidate = {
  inviter_id: number
  username: string
  display_name: string
  current_group: string
  effective_invitee_count: number
  threshold: number
  effective_top_up_amount_cents: number
  top_up_amount_threshold_cents: number
  eligible_by_invitees: boolean
  eligible_by_top_up_amount: boolean
  next_group: string
  next_rate_basis_points: number
}

export type AffiliateUpgradeNotice = {
  id: number
  inviter_id: number
  threshold: number
  effective_invitee_count: number
  top_up_amount_threshold_cents: number
  effective_top_up_amount_cents: number
  attempt_count: number
  last_attempt_time: number
  next_attempt_time: number
  dead_letter_time: number
  last_error: string
  created_time: number
  sent_time: number
}

export const AFFILIATE_PAYOUT_STATUS = {
  PENDING: 1,
  APPROVED: 2,
  PAID: 3,
  REJECTED: 4,
  CANCELLED: 5,
  PROCESSING: 6,
} as const

export type AffiliatePayoutStatus =
  (typeof AFFILIATE_PAYOUT_STATUS)[keyof typeof AFFILIATE_PAYOUT_STATUS]

export type AffiliatePayout = {
  id: number
  user_id: number
  request_id: string
  amount_cents: number
  amount_quota: number
  payment_method: 'alipay' | 'bank'
  account_name: string
  account: string
  account_last4: string
  status: AffiliatePayoutStatus
  eligible_settlement_time: number
  reject_reason?: string
  payment_reference?: string
  disbursement_mode?: 'manual' | 'alipay_direct'
  provider_order_id?: string
  provider_fund_order_id?: string
  provider_status?: string
  provider_error_code?: string
  provider_error_message?: string
  payment_attempt?: number
  processing_time?: number
  operator_id?: number
  reviewed_time?: number
  paid_time?: number
  cancelled_time?: number
  created_time: number
  updated_time: number
  username?: string
  display_name?: string
}

export type AffiliatePayoutSummary = {
  available_quota: number
  available_cents: number
  frozen_quota: number
  frozen_cents: number
  minimum_cents: number
  settlement_day: number
  next_settlement_time: number
  is_settlement_day: boolean
}

export type AffiliatePayoutAdminSummary = {
  pending_count: number
  approved_count: number
  processing_count: number
  pending_cents: number
  approved_cents: number
  processing_cents: number
  paid_cents: number
  settlement_day: number
  is_settlement_day: boolean
}

export type AffiliateAlipayPayoutProviderStatus = {
  enabled: boolean
  configured: boolean
}

export type AffiliateAlipayPayoutSettings =
  AffiliateAlipayPayoutProviderStatus & {
    app_id: string
    transfer_title: string
    private_key_configured: boolean
    app_certificate_configured: boolean
    alipay_public_certificate_configured: boolean
    alipay_root_certificate_configured: boolean
  }

export type UpdateAffiliateAlipayPayoutSettingsPayload = {
  enabled: boolean
  app_id: string
  private_key: string
  app_certificate: string
  alipay_public_certificate: string
  alipay_root_certificate: string
  transfer_title: string
  clear_keys?: boolean
}

export type AffiliatePayoutCreatePayload = {
  request_id: string
  amount_cents: number
  payment_method: 'alipay'
  account_name: string
  account: string
}

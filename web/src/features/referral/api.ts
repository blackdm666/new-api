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
import { t } from 'i18next'

import { api } from '@/lib/api'
import { UserFacingError } from '@/lib/user-facing-error'

import type {
  AffiliateAdminSummary,
  AffiliateCommission,
  AffiliateCommissionStatus,
  AffiliateInviteeStats,
  AffiliateAlipayPayoutProviderStatus,
  AffiliateAlipayPayoutSettings,
  AffiliatePayout,
  AffiliatePayoutAdminSummary,
  AffiliatePayoutCreatePayload,
  AffiliatePayoutStatus,
  AffiliatePayoutSummary,
  AffiliateSummary,
  AffiliateTransfer,
  AffiliateUpgradeCandidate,
  AffiliateUpgradeNotice,
  ApiResponse,
  PaginatedResponse,
  UpdateAffiliateAlipayPayoutSettingsPayload,
} from './types'

function unwrap<T>(response: ApiResponse<T>, fallback = 'Request failed'): T {
  if (!response.success) {
    throw new UserFacingError(response.message?.trim() || t(fallback))
  }
  return response.data as T
}

function pageQuery(page: number, pageSize: number): URLSearchParams {
  return new URLSearchParams({
    p: String(page),
    page_size: String(pageSize),
  })
}

export async function fetchAffiliateSummary(): Promise<AffiliateSummary> {
  const response = await api.get<ApiResponse<AffiliateSummary>>(
    '/api/user/aff/summary'
  )
  return unwrap(response.data)
}

export async function fetchAffiliateCommissions(
  page: number,
  pageSize: number,
  status?: AffiliateCommissionStatus | 'all'
): Promise<PaginatedResponse<AffiliateCommission>> {
  const query = pageQuery(page, pageSize)
  if (status && status !== 'all') query.set('status', String(status))
  const response = await api.get<
    ApiResponse<PaginatedResponse<AffiliateCommission>>
  >(`/api/user/aff/commissions?${query.toString()}`)
  return unwrap(response.data)
}

export async function fetchAffiliateInviteeStats(
  page: number,
  pageSize: number
): Promise<PaginatedResponse<AffiliateInviteeStats>> {
  const query = pageQuery(page, pageSize)
  const response = await api.get<
    ApiResponse<PaginatedResponse<AffiliateInviteeStats>>
  >(`/api/user/aff/invitee_stats?${query.toString()}`)
  return unwrap(response.data)
}

export async function fetchAffiliateTransfers(
  page: number,
  pageSize: number
): Promise<PaginatedResponse<AffiliateTransfer>> {
  const query = pageQuery(page, pageSize)
  const response = await api.get<
    ApiResponse<PaginatedResponse<AffiliateTransfer>>
  >(`/api/user/aff/transfers?${query.toString()}`)
  return unwrap(response.data)
}

export async function transferAffiliateCommission(payload: {
  amount_cents: number
  request_id: string
}): Promise<AffiliateTransfer> {
  const response = await api.post<ApiResponse<AffiliateTransfer>>(
    '/api/user/aff/transfers',
    payload
  )
  return unwrap(response.data)
}

export async function fetchAdminAffiliateSummary(): Promise<AffiliateAdminSummary> {
  const response = await api.get<ApiResponse<AffiliateAdminSummary>>(
    '/api/affiliate/admin/summary'
  )
  return unwrap(response.data)
}

export async function fetchAdminAffiliateCommissions(params: {
  page: number
  pageSize: number
  status?: AffiliateCommissionStatus | 'all'
  keyword?: string
}): Promise<PaginatedResponse<AffiliateCommission>> {
  const query = pageQuery(params.page, params.pageSize)
  if (params.status && params.status !== 'all') {
    query.set('status', String(params.status))
  }
  if (params.keyword?.trim()) query.set('keyword', params.keyword.trim())
  const response = await api.get<
    ApiResponse<PaginatedResponse<AffiliateCommission>>
  >(`/api/affiliate/admin/commissions?${query.toString()}`)
  return unwrap(response.data)
}

export async function fetchAdminAffiliateTransfers(params: {
  page: number
  pageSize: number
  keyword?: string
}): Promise<PaginatedResponse<AffiliateTransfer>> {
  const query = pageQuery(params.page, params.pageSize)
  if (params.keyword?.trim()) query.set('keyword', params.keyword.trim())
  const response = await api.get<
    ApiResponse<PaginatedResponse<AffiliateTransfer>>
  >(`/api/affiliate/admin/transfers?${query.toString()}`)
  return unwrap(response.data)
}

export async function approveAffiliateCommission(id: number): Promise<void> {
  const response = await api.post<ApiResponse<unknown>>(
    `/api/affiliate/admin/commissions/${id}/approve`
  )
  unwrap(response.data)
}

export async function rejectAffiliateCommission(
  id: number,
  reason = ''
): Promise<void> {
  const response = await api.post<ApiResponse<unknown>>(
    `/api/affiliate/admin/commissions/${id}/reject`,
    { reason }
  )
  unwrap(response.data)
}

export type AffiliateSettingsPayload = {
  enabled: boolean
  auto_approve: boolean
  default_rate_basis_points: number
  group_rates: Record<string, number>
  upgrade_invitees_threshold: number
  gold_upgrade_invitees_threshold: number
  upgrade_top_up_amount_threshold_cents: number
  gold_upgrade_top_up_amount_threshold_cents: number
}

export async function updateAffiliateSettings(
  payload: AffiliateSettingsPayload
): Promise<void> {
  const response = await api.put<ApiResponse<unknown>>(
    '/api/affiliate/root/settings',
    payload
  )
  unwrap(response.data)
}

export async function fetchAffiliateUpgradeCandidates(
  page = 1,
  pageSize = 20
): Promise<PaginatedResponse<AffiliateUpgradeCandidate>> {
  const query = pageQuery(page, pageSize)
  const response = await api.get<
    ApiResponse<PaginatedResponse<AffiliateUpgradeCandidate>>
  >(`/api/affiliate/admin/upgrade-candidates?${query.toString()}`)
  return unwrap(response.data)
}

export async function approveAffiliateUpgrade(
  inviterId: number,
  nextGroup: string
): Promise<void> {
  const response = await api.post<ApiResponse<unknown>>(
    `/api/affiliate/admin/upgrade-candidates/${inviterId}/approve`,
    { next_group: nextGroup }
  )
  unwrap(response.data)
}

export async function fetchAffiliateNotificationFailures(
  page = 1,
  pageSize = 20
): Promise<PaginatedResponse<AffiliateUpgradeNotice>> {
  const query = pageQuery(page, pageSize)
  const response = await api.get<
    ApiResponse<PaginatedResponse<AffiliateUpgradeNotice>>
  >(`/api/affiliate/admin/notification-failures?${query.toString()}`)
  return unwrap(response.data)
}

export async function retryAffiliateNotification(id: number): Promise<void> {
  const response = await api.post<ApiResponse<unknown>>(
    `/api/affiliate/admin/notification-failures/${id}/retry`
  )
  unwrap(response.data)
}

export async function fetchAffiliatePayoutSummary(): Promise<AffiliatePayoutSummary> {
  const response = await api.get<ApiResponse<AffiliatePayoutSummary>>(
    '/api/user/aff/payouts/summary'
  )
  return unwrap(response.data)
}

export async function fetchAffiliatePayouts(
  page: number,
  pageSize: number,
  status?: AffiliatePayoutStatus | 'all'
): Promise<PaginatedResponse<AffiliatePayout>> {
  const query = pageQuery(page, pageSize)
  if (status && status !== 'all') query.set('status', String(status))
  const response = await api.get<
    ApiResponse<PaginatedResponse<AffiliatePayout>>
  >(`/api/user/aff/payouts?${query.toString()}`)
  return unwrap(response.data)
}

export async function createAffiliatePayout(
  payload: AffiliatePayoutCreatePayload
): Promise<AffiliatePayout> {
  const response = await api.post<ApiResponse<AffiliatePayout>>(
    '/api/user/aff/payouts',
    payload
  )
  return unwrap(response.data)
}

export async function cancelAffiliatePayout(id: number): Promise<void> {
  const response = await api.post<ApiResponse<unknown>>(
    `/api/user/aff/payouts/${id}/cancel`
  )
  unwrap(response.data)
}

export async function fetchAdminAffiliatePayoutSummary(): Promise<AffiliatePayoutAdminSummary> {
  const response = await api.get<ApiResponse<AffiliatePayoutAdminSummary>>(
    '/api/affiliate/admin/payouts/summary'
  )
  return unwrap(response.data)
}

export async function fetchAdminAffiliatePayouts(params: {
  page: number
  pageSize: number
  status?: AffiliatePayoutStatus | 'all'
  keyword?: string
}): Promise<PaginatedResponse<AffiliatePayout>> {
  const query = pageQuery(params.page, params.pageSize)
  if (params.status && params.status !== 'all') {
    query.set('status', String(params.status))
  }
  if (params.keyword?.trim()) query.set('keyword', params.keyword.trim())
  const response = await api.get<
    ApiResponse<PaginatedResponse<AffiliatePayout>>
  >(`/api/affiliate/admin/payouts?${query.toString()}`)
  return unwrap(response.data)
}

export async function approveAffiliatePayout(id: number): Promise<void> {
  const response = await api.post<ApiResponse<unknown>>(
    `/api/affiliate/admin/payouts/${id}/approve`
  )
  unwrap(response.data)
}

export async function rejectAffiliatePayout(
  id: number,
  reason: string
): Promise<void> {
  const response = await api.post<ApiResponse<unknown>>(
    `/api/affiliate/admin/payouts/${id}/reject`,
    { reason }
  )
  unwrap(response.data)
}

export async function markAffiliatePayoutPaid(id: number): Promise<void> {
  const response = await api.post<ApiResponse<unknown>>(
    `/api/affiliate/admin/payouts/${id}/paid`
  )
  unwrap(response.data)
}

export async function fetchAffiliateAlipayPayoutProviderStatus(): Promise<AffiliateAlipayPayoutProviderStatus> {
  const response = await api.get<
    ApiResponse<AffiliateAlipayPayoutProviderStatus>
  >('/api/affiliate/admin/payout-provider')
  return unwrap(response.data)
}

export async function payAffiliatePayoutWithAlipay(
  id: number
): Promise<AffiliatePayout> {
  const response = await api.post<ApiResponse<AffiliatePayout>>(
    `/api/affiliate/admin/payouts/${id}/alipay`,
    undefined,
    { skipBusinessError: true }
  )
  return unwrap(response.data)
}

export async function refreshAffiliatePayoutAlipayStatus(
  id: number
): Promise<AffiliatePayout> {
  const response = await api.post<ApiResponse<AffiliatePayout>>(
    `/api/affiliate/admin/payouts/${id}/alipay/status`,
    undefined,
    { skipBusinessError: true }
  )
  return unwrap(response.data)
}

export async function fetchAffiliateAlipayPayoutSettings(): Promise<AffiliateAlipayPayoutSettings> {
  const response = await api.get<ApiResponse<AffiliateAlipayPayoutSettings>>(
    '/api/affiliate/root/payout-settings'
  )
  return unwrap(response.data)
}

export async function updateAffiliateAlipayPayoutSettings(
  payload: UpdateAffiliateAlipayPayoutSettingsPayload
): Promise<void> {
  const response = await api.put<ApiResponse<unknown>>(
    '/api/affiliate/root/payout-settings',
    payload
  )
  unwrap(response.data)
}

export async function testAffiliateAlipayPayoutSettings(): Promise<void> {
  const response = await api.post<ApiResponse<unknown>>(
    '/api/affiliate/root/payout-settings/test'
  )
  unwrap(response.data)
}

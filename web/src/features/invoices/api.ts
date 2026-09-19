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
  CreateInvoiceRequestPayload,
  EligibleTopUpOrder,
  InvoiceFile,
  InvoiceConfig,
  InvoiceListResponse,
  InvoiceMaintenance,
  InvoiceRequestDetail,
  InvoiceStorageKey,
  InvoiceStorageReconcileReport,
  InvoiceStatus,
  InvoiceUserProfile,
} from './types'

type ApiResponse<T> = {
  success: boolean
  message?: string
  data: T
}

export type InvoiceListParams = {
  page: number
  pageSize: number
  status?: InvoiceStatus | ''
  keyword?: string
}

export function buildInvoiceListQuery(params: InvoiceListParams): string {
  const search = new URLSearchParams()
  search.set('p', String(params.page))
  search.set('page_size', String(params.pageSize))
  if (params.status !== undefined && params.status !== '') {
    search.set('status', String(params.status))
  }
  if (params.keyword?.trim()) search.set('keyword', params.keyword.trim())
  return search.toString()
}

function unwrap<T>(response: ApiResponse<T>, fallback = 'Request failed'): T {
  if (!response.success) {
    throw new UserFacingError(response.message?.trim() || t(fallback))
  }
  return response.data
}

const invoiceRequestConfig = {
  skipBusinessError: true,
  skipErrorHandler: true,
} as const

// Invoice routes were introduced after some deployments had already cached
// their 404 responses. A per-page-load query value makes rolling upgrades
// recover immediately without defeating the HTTP client's in-flight request
// deduplication. The backend also marks live invoice responses non-cacheable.
const invoiceCacheVersion = Date.now().toString()

function fresh(url: string): string {
  const separator = url.includes('?') ? '&' : '?'
  return `${url}${separator}_=${invoiceCacheVersion}`
}

export async function fetchInvoiceRequests(
  params: InvoiceListParams,
  admin: boolean
): Promise<InvoiceListResponse> {
  const base = admin ? '/api/invoice/admin/requests' : '/api/invoice/requests'
  const response = await api.get<ApiResponse<InvoiceListResponse>>(
    fresh(`${base}?${buildInvoiceListQuery(params)}`),
    invoiceRequestConfig
  )
  return unwrap(response.data)
}

export async function fetchInvoiceRequest(
  id: number,
  admin: boolean
): Promise<InvoiceRequestDetail> {
  const base = admin ? '/api/invoice/admin/requests' : '/api/invoice/requests'
  const response = await api.get<ApiResponse<InvoiceRequestDetail>>(
    fresh(`${base}/${id}`),
    invoiceRequestConfig
  )
  return unwrap(response.data)
}

export async function fetchEligibleInvoiceOrders(): Promise<
  EligibleTopUpOrder[]
> {
  const response = await api.get<ApiResponse<EligibleTopUpOrder[] | null>>(
    fresh('/api/invoice/eligible_orders'),
    invoiceRequestConfig
  )
  return unwrap(response.data) ?? []
}

export async function fetchInvoiceConfig(): Promise<InvoiceConfig> {
  const response = await api.get<ApiResponse<InvoiceConfig>>(
    fresh('/api/invoice/config'),
    invoiceRequestConfig
  )
  return unwrap(response.data)
}

export async function createInvoiceRequest(
  payload: CreateInvoiceRequestPayload
): Promise<InvoiceRequestDetail> {
  const response = await api.post<ApiResponse<InvoiceRequestDetail>>(
    '/api/invoice/requests',
    payload,
    invoiceRequestConfig
  )
  return unwrap(response.data)
}

export async function updateInvoiceStatus(
  id: number,
  status: InvoiceStatus,
  rejectionReason = ''
): Promise<void> {
  const response = await api.put<ApiResponse<unknown>>(
    `/api/invoice/admin/requests/${id}/status`,
    { status, rejection_reason: rejectionReason },
    invoiceRequestConfig
  )
  unwrap(response.data)
}

export async function withdrawInvoiceRequest(id: number): Promise<void> {
  const response = await api.post<ApiResponse<unknown>>(
    `/api/invoice/requests/${id}/withdraw`,
    undefined,
    invoiceRequestConfig
  )
  unwrap(response.data)
}

export async function purgeInvoiceRequest(id: number): Promise<void> {
  const response = await api.delete<ApiResponse<unknown>>(
    `/api/invoice/admin/requests/${id}`,
    invoiceRequestConfig
  )
  unwrap(response.data)
}

export async function fetchInvoiceMaintenance(): Promise<InvoiceMaintenance> {
  const response = await api.get<ApiResponse<InvoiceMaintenance>>(
    fresh('/api/invoice/admin/maintenance'),
    invoiceRequestConfig
  )
  return unwrap(response.data)
}

export async function retryInvoiceCleanup(id: number): Promise<void> {
  const response = await api.post<ApiResponse<unknown>>(
    `/api/invoice/admin/maintenance/cleanups/${id}/retry`,
    undefined,
    invoiceRequestConfig
  )
  unwrap(response.data)
}

export async function retryInvoiceNotification(id: number): Promise<void> {
  const response = await api.post<ApiResponse<unknown>>(
    `/api/invoice/admin/maintenance/notifications/${id}/retry`,
    undefined,
    invoiceRequestConfig
  )
  unwrap(response.data)
}

export async function resendIssuedInvoiceNotification(
  invoiceId: number
): Promise<void> {
  const response = await api.post<ApiResponse<unknown>>(
    `/api/invoice/admin/requests/${invoiceId}/notifications/issued/resend`,
    undefined,
    invoiceRequestConfig
  )
  unwrap(response.data)
}

export async function reconcileInvoiceStorage(): Promise<InvoiceStorageReconcileReport> {
  const response = await api.post<ApiResponse<InvoiceStorageReconcileReport>>(
    '/api/invoice/admin/maintenance/reconcile',
    undefined,
    invoiceRequestConfig
  )
  return unwrap(response.data)
}

export async function cleanupInvoiceOrphans(
  keys: InvoiceStorageKey[]
): Promise<void> {
  const response = await api.post<ApiResponse<unknown>>(
    '/api/invoice/admin/maintenance/orphans/cleanup',
    { keys },
    invoiceRequestConfig
  )
  unwrap(response.data)
}

export async function uploadInvoiceFile(
  invoiceId: number,
  file: File
): Promise<InvoiceFile> {
  const form = new FormData()
  form.append('file', file)
  const response = await api.post<ApiResponse<InvoiceFile>>(
    `/api/invoice/admin/requests/${invoiceId}/files`,
    form,
    invoiceRequestConfig
  )
  return unwrap(response.data)
}

export async function deleteInvoiceFile(
  invoiceId: number,
  fileId: number
): Promise<void> {
  const response = await api.delete<ApiResponse<unknown>>(
    `/api/invoice/admin/requests/${invoiceId}/files/${fileId}`,
    invoiceRequestConfig
  )
  unwrap(response.data)
}

export async function downloadInvoiceFile(
  invoiceId: number,
  file: InvoiceFile,
  inline = false
): Promise<void> {
  const fileUrl = `/api/invoice/requests/${invoiceId}/files/${file.id}${inline ? '?inline=1' : ''}`
  if (inline) {
    window.open(fileUrl, '_blank', 'noopener,noreferrer')
  } else {
    const anchor = document.createElement('a')
    anchor.href = fileUrl
    anchor.download = file.file_name
    document.body.appendChild(anchor)
    anchor.click()
    anchor.remove()
  }
}

export async function fetchInvoiceUserProfile(
  invoiceId: number
): Promise<InvoiceUserProfile> {
  const response = await api.get<ApiResponse<InvoiceUserProfile>>(
    fresh(`/api/invoice/admin/requests/${invoiceId}/user-profile`),
    invoiceRequestConfig
  )
  return unwrap(response.data)
}

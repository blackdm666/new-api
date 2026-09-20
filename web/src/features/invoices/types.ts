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
export const INVOICE_STATUS = {
  PENDING: 1,
  ISSUED: 2,
  REJECTED: 3,
  WITHDRAWN: 4,
  EXPIRED: 5,
} as const

export const INVOICE_STATUS_OPTIONS = [
  INVOICE_STATUS.PENDING,
  INVOICE_STATUS.ISSUED,
  INVOICE_STATUS.REJECTED,
  INVOICE_STATUS.WITHDRAWN,
  INVOICE_STATUS.EXPIRED,
] as const

export type InvoiceStatus = (typeof INVOICE_STATUS)[keyof typeof INVOICE_STATUS]

export type InvoiceRequest = {
  id: number
  user_id: number
  username?: string
  company_name: string
  tax_number: string
  bank_name: string
  bank_account: string
  company_address: string
  company_phone: string
  remark: string
  topup_order_ids: string
  total_money: number
  total_money_cents: number
  tax_rate_basis_points: number
  tax_fee_cents: number
  tax_fee_quota: number
  status: InvoiceStatus
  rejection_reason: string
  issued_time: number
  redacted_time: number
  expiry_warning_time: number
  expires_at: number
  created_time: number
  updated_time: number
}

export type InvoiceFile = {
  id: number
  invoice_request_id: number
  uploader_id: number
  file_name: string
  mime_type: string
  size: number
  storage_type: string
  sha256: string
  previewable?: boolean
  created_time: number
}

export type EligibleTopUpOrder = {
  id: number
  trade_no: string
  payment_method: string
  amount: number
  money: number
  status: string
  create_time: number
  complete_time?: number
}

export type InvoiceRequestDetail = {
  invoice: InvoiceRequest
  orders: EligibleTopUpOrder[]
  files: InvoiceFile[]
  events: InvoiceRequestEvent[]
  notifications?: InvoiceNotificationDelivery[]
}

export type InvoiceRequestEvent = {
  id: number
  from_status: number
  to_status: InvoiceStatus
  operator_id?: number
  reason: string
  created_time: number
}

export type InvoiceConfig = {
  minimum_amount_cents: number
  tax_rate_basis_points: number
  available_balance_cents: number
  issue_day: number
}

export type InvoiceListResponse = {
  items: InvoiceRequest[]
  total: number
}

export type CreateInvoiceRequestPayload = {
  company_name: string
  tax_number: string
  bank_name?: string
  bank_account?: string
  company_address?: string
  company_phone?: string
  remark?: string
  topup_order_ids: number[]
}

export type InvoiceUserProfile = {
  user_id: number
  username: string
  display_name?: string
  email?: string
  role: number
  status: number
  group?: string
  created_time?: number
  quota?: number
  used_quota?: number
  request_count?: number
  recent_logs?: InvoiceUserRecentLog[]
  model_usage?: InvoiceUserModelUsage[]
}

export type InvoiceUserRecentLog = {
  id: number
  created_at: number
  model_name: string
  token_name: string
  quota: number
  prompt_tokens: number
  completion_tokens: number
}

export type InvoiceUserModelUsage = {
  model_name: string
  count: number
  quota: number
  token_used: number
}

export type InvoiceFileCleanup = {
  id: number
  storage_profile_id: number
  storage_type: string
  storage_key: string
  attempts: number
  last_error: string
  next_attempt_time: number
  locked_until: number
  created_time: number
}

export type InvoiceFileUpload = {
  id: string
  invoice_request_id: number
  uploader_id: number
  storage_profile_id: number
  storage_type: string
  storage_key: string
  file_name: string
  mime_type: string
  size: number
  created_time: number
}

export type InvoiceStorageProfile = {
  id: number
  storage_type: string
  created_time: number
}

export type InvoiceNotificationDelivery = {
  id: number
  invoice_request_id: number
  kind: string
  recipient: string
  attempts: number
  last_error: string
  next_attempt_time: number
  locked_until: number
  delivered_time: number
  created_time: number
  updated_time: number
}

export type InvoiceMaintenance = {
  cleanups: InvoiceFileCleanup[]
  uploads: InvoiceFileUpload[]
  profiles: InvoiceStorageProfile[]
  notifications: InvoiceNotificationDelivery[]
}

export type InvoiceStorageKey = {
  storage_profile_id: number
  storage_type: string
  storage_key: string
}

export type InvoiceStorageReconcileReport = {
  profiles: Array<{
    storage_profile_id: number
    storage_type: string
    objects_scanned: number
    truncated: boolean
    error?: string
  }>
  orphan_keys: InvoiceStorageKey[]
  missing_files: InvoiceStorageKey[]
}

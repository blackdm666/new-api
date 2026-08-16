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
export type MarketingLocalizedContent = {
  subject: string
  body: string
}

export type MarketingAudienceRule = {
  groups?: string[]
  inactive_days?: number
  topup_count_min?: number
  topup_count_max?: number
  last_topup_before?: number
  quota_min?: number
  quota_max?: number
  used_quota_positive?: boolean
}

export type MarketingCampaign = {
  id: number
  name: string
  scene: string
  status: string
  audience_rule: string
  localized_content: string
  scheduled_time: number
  paused_reason: string
  recipient_count: number
  delivered_count: number
  failed_count: number
  clicked_count: number
  converted_count: number
  converted_cents: number
  created_time: number
}

export type MarketingAutomation = {
  id: number
  scene: string
  enabled: boolean
  apply_existing: boolean
  baseline_ready: boolean
  localized_content: string
  updated_time: number
}

export type MarketingRecipient = {
  id: number
  username: string
  recipient_masked: string
  language: string
  status: string
  delivered_time: number
  clicked_time: number
  converted_time: number
  last_error: string
  created_time: number
}

export type MarketingSuppression = {
  id: number
  user_id: number
  email_masked: string
  reason: string
  created_time: number
}

export type MarketingOverview = {
  campaigns: number
  queued: number
  delivered: number
  failed: number
  clicked: number
  converted: number
  converted_cents: number
}

export type Paginated<T> = {
  items: T[]
  total: number
}

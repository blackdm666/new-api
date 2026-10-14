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
// 88api cyberguard: response shapes of the root-only /api/cyber-guard API.
// All times are unix seconds.

export type CyberGuardMode = 'off' | 'observe' | 'enforce'
export type CyberGuardCategory = 'cyber' | 'bio' | 'usage' | 'session_blocked'
export type CyberGuardEventState =
  | 'pending'
  | 'counted'
  | 'record_only'
  | 'unattributed'
  | 'exempt'
  | 'void'
export type CyberGuardBanStatus = 'observed' | 'active' | 'ended' | 'dismissed'
export type CyberGuardBanSource = 'auto' | 'manual' | 'adopted'
export type CyberGuardUserStatus = 'all' | 'active' | 'observed' | 'clean'

export type ApiEnvelope<T> = { success: boolean; message?: string; data: T }

export type Paginated<T> = {
  page: number
  page_size: number
  total: number
  items: T[] | null
}

export type CyberGuardOverview = {
  mode: CyberGuardMode
  is_master_node: boolean
  last_poll: {
    at: number
    ok: boolean
    /** partial: a catch-up poll read part of the backlog; the next run continues. */
    status: '' | 'ok' | 'partial' | 'error' | 'skipped'
    error: string
    fetched: number
    new_events: number
    cursor_end: number
  }
  counts: {
    hits_today: number
    hits_7d: number
    units_7d: number
    users_flagged_7d: number
    active_bans: number
    observed_open: number
    unattributed_7d: number
    pending: number
  }
  reconciliation_24h: {
    sub2api_rows: number
    attributed: number
    unattributed: number
    pending: number
  }
}

export type CyberGuardUserRow = {
  user_id: number
  username: string
  email: string
  current_group: string
  role: number
  units_7d: number
  units_30d: number
  hits_30d: number
  last_hit_at: number
  ban: {
    id: number
    case_id: string
    status: 'active' | 'observed'
    source: CyberGuardBanSource
    expires_at: number
  } | null
  is_reseller: boolean
  /** True when the user id is in the exempt_user_ids setting. */
  is_exempt?: boolean
}

export type CyberGuardEvent = {
  id: number
  ops_id: number
  occurred_at: number
  sub2api_request_id: string
  api_key_id: number
  account_id: number
  account_name: string
  category: CyberGuardCategory
  error_type: string
  status_code: number
  endpoint: string
  model: string
  message: string
  user_id: number
  username: string
  token_id: number
  token_name: string
  token_group: string
  channel_id: number
  log_id: number
  session_fp: string
  unit_key: string
  is_new_unit: boolean
  state: CyberGuardEventState
  void_reason: string
  voided_by: number
  voided_at: number
  created_at: number
}

export type CyberGuardBan = {
  id: number
  case_id: string
  user_id: number
  username: string
  original_group: string
  restore_group: string
  ban_group: string
  source: CyberGuardBanSource
  status: CyberGuardBanStatus
  tier_index: number
  ban_hours: number
  units_summary: string
  reason: string
  started_at: number
  expires_at: number
  ended_at: number
  end_reason: string
  end_note: string
  ended_by: number
  created_by: number
  email_state: string
  email_delivery_key: string
  email_subject: string
  email_error: string
  created_at: number
  updated_at: number
}

export type CyberGuardBanRow = CyberGuardBan & {
  current_group: string
  email_delivery_state: string
}

export type CyberGuardUnbanResult = {
  ban: CyberGuardBan
  from_group: string
  to_group: string
  group_changed: boolean
}

export type CyberGuardAdoptableUser = {
  user_id: number
  username: string
  remark: string
  suggested_original_group: string
}

export type CyberGuardAccountRow = {
  account_id: number
  account_name: string
  hits_cyber: number
  hits_bio: number
  hits_usage: number
  users: number
  last_hit_at: number
  status: string
  schedulable: boolean | null
  error_message: string
}

export type CyberGuardTier = {
  window_days: number
  min_units: number
  ban_hours: number
}

export type CyberGuardSettings = {
  mode: CyberGuardMode
  protected_channel_ids: number[]
  sub2api_base_url: string
  sub2api_api_key_ids: number[]
  poll_interval_seconds: number
  lookback_minutes: number
  count_categories: string[]
  nosession_merge_seconds: number
  tiers: CyberGuardTier[]
  ban_group: string
  exempt_user_ids: number[]
  email_enabled: boolean
  notify_root: boolean
  has_sub2api_admin_api_key: boolean
}

/** PUT body; an omitted or empty key keeps the stored sub2api admin key. */
export type CyberGuardSettingsUpdate = Omit<
  CyberGuardSettings,
  'has_sub2api_admin_api_key'
> & { sub2api_admin_api_key?: string }

export type CyberGuardPollResult = {
  status: 'ok' | 'partial' | 'error' | 'skipped'
  error?: string
  window_start: number
  window_end: number
  fetched: number
  new_events: number
  attributed: number
  unattributed: number
  counted: number
  bans_applied: number
  proposals: number
  escalated: number
  expired: number
  drift_reapplied: number
}

export type CyberGuardEventQuery = {
  user_id?: number
  category?: string
  state?: string
  keyword?: string
  start?: number
  p: number
  page_size: number
}

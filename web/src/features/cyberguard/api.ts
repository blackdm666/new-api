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
import { queryOptions } from '@tanstack/react-query'

import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

import type {
  ApiEnvelope,
  CyberGuardAccountRow,
  CyberGuardAdoptableUser,
  CyberGuardBan,
  CyberGuardBanRow,
  CyberGuardEvent,
  CyberGuardEventQuery,
  CyberGuardOverview,
  CyberGuardPollResult,
  CyberGuardSettings,
  CyberGuardSettingsUpdate,
  CyberGuardUnbanResult,
  CyberGuardUserRow,
  CyberGuardUserStatus,
  Paginated,
} from './types'

const BASE = '/api/cyber-guard'

/** Root query key; every mutation invalidates the whole page. */
export const CYBERGUARD_QUERY_KEY = ['cyberguard'] as const

function unwrap<T>(response: { data: ApiEnvelope<T> }): T {
  return requireServerSuccess(response.data).data
}

export async function fetchCyberGuardOverview(): Promise<CyberGuardOverview> {
  return unwrap(await api.get(`${BASE}/overview`))
}

/** Shared by the page header (mode badge) and the overview tab. */
export const cyberGuardOverviewQuery = queryOptions({
  queryKey: [...CYBERGUARD_QUERY_KEY, 'overview'],
  queryFn: fetchCyberGuardOverview,
  refetchInterval: 30_000,
})

export async function fetchCyberGuardUsers(params: {
  keyword: string
  status: CyberGuardUserStatus
  p: number
  page_size: number
}): Promise<Paginated<CyberGuardUserRow>> {
  return unwrap(await api.get(`${BASE}/users`, { params }))
}

export async function fetchCyberGuardEvents(
  params: CyberGuardEventQuery
): Promise<Paginated<CyberGuardEvent>> {
  return unwrap(await api.get(`${BASE}/events`, { params }))
}

export async function voidCyberGuardEvent(
  id: number,
  reason: string
): Promise<CyberGuardEvent> {
  return unwrap(await api.post(`${BASE}/events/${id}/void`, { reason }))
}

export async function fetchCyberGuardBans(params: {
  status?: string
  user_id?: number
  p: number
  page_size: number
}): Promise<Paginated<CyberGuardBanRow>> {
  return unwrap(await api.get(`${BASE}/bans`, { params }))
}

export async function createCyberGuardBan(payload: {
  user_id: number
  reason: string
  hours: number
}): Promise<CyberGuardBan> {
  return unwrap(await api.post(`${BASE}/bans`, payload))
}

export async function unbanCyberGuardCase(
  id: number,
  reason: string
): Promise<CyberGuardUnbanResult> {
  return unwrap(await api.post(`${BASE}/bans/${id}/unban`, { reason }))
}

export async function promoteCyberGuardCase(
  id: number
): Promise<CyberGuardBan> {
  return unwrap(await api.post(`${BASE}/bans/${id}/promote`))
}

export async function dismissCyberGuardCase(
  id: number,
  reason: string
): Promise<CyberGuardBan> {
  return unwrap(await api.post(`${BASE}/bans/${id}/dismiss`, { reason }))
}

export async function resendCyberGuardBanEmail(
  id: number
): Promise<CyberGuardBan> {
  return unwrap(await api.post(`${BASE}/bans/${id}/resend-email`))
}

export async function fetchCyberGuardAdoptable(): Promise<
  CyberGuardAdoptableUser[]
> {
  return unwrap(await api.get(`${BASE}/adoptable`)) ?? []
}

export async function adoptCyberGuardBan(payload: {
  user_id: number
  original_group: string
  reason: string
  hours: number
}): Promise<CyberGuardBan> {
  return unwrap(await api.post(`${BASE}/adopt`, payload))
}

export async function fetchCyberGuardAccounts(
  days: 7 | 30
): Promise<CyberGuardAccountRow[]> {
  return unwrap(await api.get(`${BASE}/accounts`, { params: { days } })) ?? []
}

export async function fetchCyberGuardSettings(): Promise<CyberGuardSettings> {
  return unwrap(await api.get(`${BASE}/settings`))
}

export async function updateCyberGuardSettings(
  payload: CyberGuardSettingsUpdate
): Promise<CyberGuardSettings> {
  return unwrap(await api.put(`${BASE}/settings`, payload))
}

export async function pollCyberGuardNow(): Promise<CyberGuardPollResult> {
  return unwrap(await api.post(`${BASE}/poll-now`))
}

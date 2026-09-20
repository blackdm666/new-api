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
import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

import type { NoticeRecord, NoticeSubmission, NoticeUser } from './types'

function endpoint(preview: boolean) {
  if (!preview) return '/api/marketing/user-notices'
  if (!['localhost', '127.0.0.1'].includes(window.location.hostname)) {
    throw new Error('Local preview only')
  }
  return '/__notification-preview/notices'
}

function readResult<T>(response: {
  data: { success: boolean; data: T; message?: string }
}): T {
  return requireServerSuccess(response.data).data
}

export async function searchNoticeUsers(
  query: string,
  preview = false
): Promise<NoticeUser[]> {
  endpoint(preview)
  return readResult(
    await api.get(
      preview ? '/__notification-preview/users' : `${endpoint(false)}/users`,
      { params: { q: query } }
    )
  )
}

export async function listNotices(preview = false): Promise<NoticeRecord[]> {
  return readResult(await api.get(endpoint(preview)))
}

export async function submitNotice(
  payload: NoticeSubmission,
  preview = false
): Promise<NoticeRecord> {
  return readResult(await api.post(endpoint(preview), payload))
}

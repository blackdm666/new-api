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
import { z } from 'zod'

export const noticeKinds = [
  'maintenance',
  'usage',
  'violation',
  'service',
] as const
export type NoticeKind = (typeof noticeKinds)[number]

export const noticeSchema = z.object({
  kind: z.enum(noticeKinds),
  subject: z.string().trim().min(1).max(120),
  body: z.string().trim().min(1).max(5000),
})
export type NoticeContent = z.infer<typeof noticeSchema>

export type NoticeUser = {
  id: number
  username: string
  display_name: string
  email_masked: string
  group: string
  disabled: boolean
  skip_reason:
    | 'missing_email'
    | 'blocked_email'
    | 'invalid_email'
    | 'duplicate_email'
    | ''
    | null
  delivery_id?: number
  state?: string
  smtp_profile?: string
  smtp_channel?: string
  last_error?: string
}

export type NoticeRecord = NoticeContent & {
  id: number
  recipients: NoticeUser[]
  status: 'draft' | 'simulated' | 'queued'
  created_at: number
  operator: string
}

export type NoticeSubmission = NoticeContent & {
  id?: number
  user_ids: number[]
  action: 'draft' | 'send'
  request_key: string
}

export function localNoticePreviewEnabled(version?: string) {
  return (
    version === 'local-notification-preview' &&
    ['localhost', '127.0.0.1'].includes(window.location.hostname)
  )
}

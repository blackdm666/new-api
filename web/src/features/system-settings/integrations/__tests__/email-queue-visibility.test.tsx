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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { emailCategoryLabel } from '../email-category-labels'
import { EmailQueueSection } from '../email-queue-section'

function successfulResponse(data: unknown): ReturnType<typeof api.get> {
  return Promise.resolve({ data: { success: true, data } }) as ReturnType<
    typeof api.get
  >
}

function renderEmailQueue() {
  vi.spyOn(api, 'get').mockImplementation((url) => {
    if (url === '/api/option/email_deliveries') {
      return successfulResponse({
        items: [
          {
            id: 1982,
            category: 'email_verification',
            related_id: 0,
            user_id: 0,
            recipient: '2708826161@qq.com',
            recipient_masked: '21***@qq.com',
            priority: 200,
            status: 'delivered',
            sender_account_id: 1,
            sender_account_name: 'Marketing sender A',
            attempts: 0,
            last_error: '',
            next_attempt_time: 0,
            delivered_time: 1,
            dead_letter_time: 0,
            expired_time: 0,
            created_time: 1,
          },
        ],
        total: 1,
      })
    }
    if (url === '/api/option/email_deliveries/stats') {
      return successfulResponse({
        queue: {
          queued: 0,
          sending: 0,
          retrying: 0,
          awaiting_receipt: 0,
          accepted_untracked_24h: 0,
          final_delivered_24h: 0,
          failed: 0,
          delivered_24h: 1,
          failed_24h: 0,
          failure_rate_24h: 0,
          oldest_pending_time: 0,
          last_delivered_time: 1,
          marketing_quota_used_today: 0,
        },
        categories: ['email_verification'],
        smtp_configured: true,
        marketing_daily_limit: 500,
        marketing_circuit_breaker: {
          paused_campaigns: 0,
          last_reason: '',
        },
        rules: {},
      })
    }
    throw new Error(`Unexpected GET ${url}`)
  })

  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <EmailQueueSection />
    </QueryClientProvider>
  )
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('email queue recipient visibility', () => {
  test('shows the complete recipient and omits the unused related ID', async () => {
    renderEmailQueue()

    expect(await screen.findByText('2708826161@qq.com')).toBeInTheDocument()
    expect(screen.queryByText('21***@qq.com')).not.toBeInTheDocument()
    expect(screen.getByText('#1982')).toBeInTheDocument()
    expect(screen.getByText('Marketing sender A')).toBeInTheDocument()
    expect(screen.queryByText(/Related ID/)).not.toBeInTheDocument()
  })
})

describe('email queue marketing category labels', () => {
  const translations: Record<string, string> = {
    'Registration without first API request': '注册后未完成首次调用',
    'Single top-up win-back': '单次充值未复购',
    'Long-term inactive user': '长期未登录',
    'Referral program activation': '推广计划激活',
  }
  const t = (key: string) => translations[key] ?? key

  test.each([
    ['marketing_registration_no_first_call', '注册后未完成首次调用'],
    ['marketing_single_topup_winback', '单次充值未复购'],
    ['marketing_inactive_user', '长期未登录'],
    ['marketing_affiliate_program_activation', '推广计划激活'],
  ])('localizes %s', (category, expected) => {
    expect(emailCategoryLabel(category, t)).toBe(expected)
  })

  test('preserves unknown category identifiers for diagnostics', () => {
    expect(emailCategoryLabel('unknown_category', t)).toBe('unknown_category')
  })
})

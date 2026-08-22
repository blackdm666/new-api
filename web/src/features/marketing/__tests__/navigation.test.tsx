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
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { MarketingAdminPage } from '..'
import { OPERATIONS_SECTION_IDS } from '../../system-settings/operations/section-registry'

function successfulResponse(data: unknown): ReturnType<typeof api.get> {
  return Promise.resolve({ data: { success: true, data } }) as ReturnType<
    typeof api.get
  >
}

function renderMarketingPage() {
  vi.spyOn(api, 'get').mockImplementation((url) => {
    switch (url) {
      case '/api/marketing/overview':
        return successfulResponse({})
      case '/api/marketing/campaigns':
        return successfulResponse({ items: [], total: 0 })
      case '/api/marketing/automations':
        return successfulResponse([])
      case '/api/option/email_deliveries':
        return successfulResponse({ items: [], total: 0 })
      case '/api/option/email_deliveries/stats':
        return successfulResponse({
          queue: {
            queued: 0,
            sending: 0,
            retrying: 0,
            failed: 0,
            delivered_24h: 0,
            failed_24h: 0,
            failure_rate_24h: 0,
            oldest_pending_time: 0,
            last_delivered_time: 0,
            marketing_quota_used_today: 0,
          },
          categories: [],
          smtp_configured: true,
          marketing_daily_limit: 500,
          marketing_circuit_breaker: {
            paused_campaigns: 0,
            last_reason: '',
          },
          rules: {
            marketing_daily_limit: 500,
            marketing_per_minute_limit: 20,
            marketing_user_cooldown_days: 7,
            marketing_send_start_hour: 9,
            marketing_send_end_hour: 20,
            email_max_attempts: 8,
            email_retry_initial_seconds: 30,
            email_retry_max_seconds: 86400,
            delivered_retention_days: 30,
            terminal_retention_days: 90,
          },
        })
      default:
        throw new Error(`Unexpected GET ${url}`)
    }
  })

  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <MarketingAdminPage />
    </QueryClientProvider>
  )
}

describe('email marketing navigation', () => {
  test('opens the email queue inside marketing and removes its settings entry', async () => {
    renderMarketingPage()

    expect(OPERATIONS_SECTION_IDS).not.toContain('email-queue')
    expect(
      screen.getByRole('button', { name: 'Create campaign' })
    ).toBeInTheDocument()

    fireEvent.click(screen.getByRole('tab', { name: 'Email Queue' }))

    expect(
      await screen.findByRole('tab', { name: 'Queue monitoring' })
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Create campaign' })
    ).not.toBeInTheDocument()
  })
})

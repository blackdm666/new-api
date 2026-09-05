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
import userEvent from '@testing-library/user-event'
import { describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { MarketingAdminPage } from '..'
import { OPERATIONS_SECTION_IDS } from '../../system-settings/operations/section-registry'

function successfulResponse(data: unknown): ReturnType<typeof api.get> {
  return Promise.resolve({ data: { success: true, data } }) as ReturnType<
    typeof api.get
  >
}

type MarketingPageOptions = {
  automations?: Array<Record<string, unknown>>
  latestAnnouncement?: Record<string, unknown> | null
  preview?: { subject: string; body: string }
  recipients?: Array<Record<string, unknown>>
}

function renderMarketingPage(options: MarketingPageOptions = {}) {
  vi.spyOn(api, 'get').mockImplementation((url) => {
    switch (url) {
      case '/api/marketing/overview':
        return successfulResponse({})
      case '/api/marketing/campaigns':
        return successfulResponse({
          items: [
            {
              id: 2,
              name: 'Marketing launch',
              scene: 'custom',
              status: 'completed',
              created_time: 1,
              recipient_count: 1,
              delivered_count: 1,
              clicked_count: 0,
              converted_count: 0,
              converted_cents: 0,
            },
          ],
          total: 1,
        })
      case '/api/marketing/automations':
        return successfulResponse(options.automations ?? [])
      case '/api/marketing/announcements/latest':
        return successfulResponse(options.latestAnnouncement ?? null)
      case '/api/marketing/campaigns/2/recipients':
        return successfulResponse({
          items: options.recipients ?? [],
          total: options.recipients?.length ?? 0,
        })
      case '/api/option/email_deliveries':
        return successfulResponse({ items: [], total: 0 })
      case '/api/option/email_deliveries/stats':
        return successfulResponse({
          queue: {
            queued: 0,
            sending: 0,
            retrying: 0,
            awaiting_receipt: 0,
            accepted_untracked_24h: 0,
            final_delivered_24h: 0,
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
            receipt_timeout_hours: 24,
            delivered_retention_days: 30,
            terminal_retention_days: 90,
          },
        })
      default:
        throw new Error(`Unexpected GET ${url}`)
    }
  })
  vi.spyOn(api, 'post').mockImplementation((url) => {
    if (url === '/api/marketing/preview') {
      return successfulResponse(
        options.preview ?? {
          subject: 'Preview subject',
          body: '<div>Rendered marketing email</div>',
        }
      )
    }
    throw new Error(`Unexpected POST ${url}`)
  })
  vi.spyOn(api, 'put').mockImplementation(() => successfulResponse({}))

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
  test('shows queue monitoring and queue rules as separate marketing tabs', async () => {
    renderMarketingPage()

    expect(OPERATIONS_SECTION_IDS).not.toContain('email-queue')
    expect(
      screen.getByRole('button', { name: 'Create campaign' })
    ).toBeInTheDocument()

    fireEvent.click(screen.getByRole('tab', { name: 'Email Queue' }))

    expect(
      await screen.findByPlaceholderText(
        'Search by email type, user, recipient, or queue ID'
      )
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('tab', { name: 'Queue monitoring' })
    ).not.toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Create campaign' })
    ).toBeInTheDocument()
    expect(screen.getByText('Waiting for receipt')).toBeInTheDocument()
    expect(
      screen
        .getByText('Waiting for receipt')
        .compareDocumentPosition(screen.getByRole('tablist')) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()

    fireEvent.click(screen.getByRole('tab', { name: 'Email queue rules' }))

    expect(
      await screen.findByRole('button', { name: 'Save queue rules' })
    ).toBeInTheDocument()
    expect(
      screen.queryByPlaceholderText(
        'Search by email type, user, recipient, or queue ID'
      )
    ).not.toBeInTheDocument()
  })

  test('opens campaign choices in the themed select popup', async () => {
    renderMarketingPage()

    fireEvent.click(screen.getByRole('tab', { name: 'Sending records' }))
    fireEvent.click(await screen.findByRole('combobox', { name: 'Campaign' }))

    expect(
      await screen.findByRole('option', { name: 'Select campaign' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('option', { name: '#2 Marketing launch' })
    ).toBeInTheDocument()
  })

  test('filters sending records by clicked engagement on the server', async () => {
    const user = userEvent.setup()
    renderMarketingPage()

    await screen.findByText('Marketing launch')
    await user.click(screen.getByRole('tab', { name: 'Sending records' }))
    await user.click(
      await screen.findByRole('combobox', { name: 'Interaction status' })
    )
    await user.click(
      await screen.findByRole('option', { name: 'Clicked recipients' })
    )

    await vi.waitFor(() => {
      expect(api.get).toHaveBeenCalledWith(
        '/api/marketing/campaigns/2/recipients',
        expect.objectContaining({
          params: expect.objectContaining({ engagement: 'clicked' }),
        })
      )
    })
  })

  test('renders readable language and localized recipient status labels', async () => {
    const user = userEvent.setup()
    renderMarketingPage({
      recipients: [
        {
          id: 1,
          username: 'recipient_user',
          recipient_masked: 're***@example.com',
          language: 'zh-CN',
          status: 'delivered',
          delivered_time: 1,
          clicked_time: 0,
          converted_time: 0,
        },
        {
          id: 2,
          username: 'skipped_user',
          recipient_masked: 'sk***@example.com',
          language: 'en',
          status: 'skipped',
          delivered_time: 0,
          clicked_time: 0,
          converted_time: 0,
        },
      ],
    })

    await screen.findByText('Marketing launch')
    await user.click(screen.getByRole('tab', { name: 'Sending records' }))

    expect(await screen.findByText('简体中文')).toBeInTheDocument()
    expect(screen.getByText('English')).toBeInTheDocument()
    expect(screen.getByText('Delivered')).toBeInTheDocument()
    expect(screen.getByText('Skipped')).toBeInTheDocument()
    expect(screen.queryByText('zh-CN')).not.toBeInTheDocument()
    expect(screen.queryByText('delivered')).not.toBeInTheDocument()
  })

  test('inserts the latest announcement into announcement email content', async () => {
    const user = userEvent.setup()
    renderMarketingPage({
      automations: [
        {
          id: 5,
          scene: 'announcement',
          enabled: false,
          apply_existing: false,
          baseline_ready: true,
          localized_content: JSON.stringify({
            'zh-CN': { subject: 'New notice', body: 'Intro copy' },
            en: { subject: 'New notice', body: 'Intro copy' },
          }),
          updated_time: 1,
        },
      ],
      latestAnnouncement: {
        id: 9,
        content: 'Latest announcement',
        extra: 'Additional details',
        publish_date: '2026-08-23T08:00:00Z',
      },
    })

    await screen.findByText('Marketing launch')
    await user.click(screen.getByRole('tab', { name: 'Automations' }))
    await user.click(
      await screen.findByRole('button', { name: 'Configure automation' })
    )
    const body = screen.getByDisplayValue('Intro copy')
    await user.click(
      screen.getByRole('button', { name: 'Insert latest announcement' })
    )

    await vi.waitFor(() => {
      expect(body).toHaveValue('Latest announcement\n\nAdditional details')
    })
  })

  test('previews the actual rendered announcement email template', async () => {
    const user = userEvent.setup()
    renderMarketingPage({
      automations: [
        {
          id: 5,
          scene: 'announcement',
          enabled: false,
          apply_existing: false,
          baseline_ready: true,
          localized_content: JSON.stringify({
            'zh-CN': { subject: 'New notice', body: 'Announcement body' },
            en: { subject: 'New notice', body: 'Announcement body' },
          }),
          updated_time: 1,
        },
      ],
      preview: {
        subject: 'New notice',
        body: '<div>Actual rendered announcement template</div>',
      },
    })

    await screen.findByText('Marketing launch')
    await user.click(screen.getByRole('tab', { name: 'Automations' }))
    await user.click(
      await screen.findByRole('button', { name: 'Configure automation' })
    )
    await user.click(screen.getByRole('button', { name: 'Preview email' }))

    const preview = await screen.findByTitle('marketing-email-preview')
    expect(preview).toHaveAttribute(
      'srcdoc',
      '<div>Actual rendered announcement template</div>'
    )
    expect(api.post).toHaveBeenCalledWith(
      '/api/marketing/preview',
      expect.objectContaining({
        scene: 'announcement',
        language: 'zh-CN',
      })
    )
  })

  test('hides retired balance automation scenes', async () => {
    const automation = (scene: string) => ({
      id: scene.length,
      scene,
      enabled: false,
      apply_existing: false,
      baseline_ready: true,
      localized_content: JSON.stringify({
        'zh-CN': { subject: scene, body: scene },
        en: { subject: scene, body: scene },
      }),
      updated_time: 1,
    })
    renderMarketingPage({
      automations: [
        automation('registration_no_first_call'),
        automation('single_topup_winback'),
        automation('paid_low_balance'),
        automation('trial_low_balance'),
        automation('inactive_user'),
        automation('affiliate_program_activation'),
        automation('announcement'),
      ],
    })

    await screen.findByText('Marketing launch')
    fireEvent.click(screen.getByRole('tab', { name: 'Automations' }))

    expect(
      await screen.findByText('Single top-up win-back')
    ).toBeInTheDocument()
    expect(
      screen.getByText('Registration without first API request')
    ).toBeInTheDocument()
    expect(screen.getByText('Long-term inactive user')).toBeInTheDocument()
    expect(screen.getByText('Referral program activation')).toBeInTheDocument()
    expect(screen.getByText('New announcement')).toBeInTheDocument()
    expect(screen.queryByText('Paid user low balance')).not.toBeInTheDocument()
    expect(
      screen.queryByText('Trial balance almost depleted')
    ).not.toBeInTheDocument()
  })

  test('saves customized runtime logic for a retained automation', async () => {
    const user = userEvent.setup()
    renderMarketingPage({
      automations: [
        {
          id: 1,
          scene: 'single_topup_winback',
          enabled: false,
          apply_existing: false,
          baseline_ready: true,
          trigger_config: JSON.stringify({
            match_days: 30,
            max_sends_per_user: 1,
            repeat_interval_days: 30,
          }),
          localized_content: JSON.stringify({
            'zh-CN': { subject: 'Win back', body: 'Come back' },
            en: { subject: 'Win back', body: 'Come back' },
          }),
          updated_time: 1,
        },
      ],
    })

    await screen.findByText('Marketing launch')
    await user.click(screen.getByRole('tab', { name: 'Automations' }))
    await user.click(
      await screen.findByRole('button', { name: 'Configure automation' })
    )
    expect(
      screen.getByRole('combobox', { name: 'Current editing language' })
    ).toBeInTheDocument()
    expect(
      screen.getByText(
        "Emails follow each recipient's language. If a template is missing, Simplified Chinese is used."
      )
    ).toBeInTheDocument()
    const maxSends = screen.getByRole('spinbutton', {
      name: 'Maximum sends per user',
    })
    await user.clear(maxSends)
    await user.type(maxSends, '2')
    await user.click(screen.getByRole('button', { name: 'Save automation' }))

    await vi.waitFor(() => {
      expect(api.put).toHaveBeenCalledWith(
        '/api/marketing/automations/single_topup_winback',
        expect.objectContaining({
          trigger_config: expect.objectContaining({
            match_days: 30,
            max_sends_per_user: 2,
            repeat_interval_days: 30,
          }),
        })
      )
    })
  })

  test('saves a 30-minute registration wait', async () => {
    const user = userEvent.setup()
    renderMarketingPage({
      automations: [
        {
          id: 1,
          scene: 'registration_no_first_call',
          enabled: false,
          apply_existing: false,
          baseline_ready: true,
          trigger_config: JSON.stringify({
            registration_wait_hours: 1,
            max_sends_per_user: 1,
            repeat_interval_days: 2,
          }),
          localized_content: JSON.stringify({
            'zh-CN': { subject: 'First call', body: 'Start using the API' },
            en: { subject: 'First call', body: 'Start using the API' },
          }),
          updated_time: 1,
        },
      ],
    })

    await screen.findByText('Marketing launch')
    await user.click(screen.getByRole('tab', { name: 'Automations' }))
    await user.click(
      await screen.findByRole('button', { name: 'Configure automation' })
    )
    const waitHours = screen.getByRole('spinbutton', {
      name: 'Wait after registration (hours)',
    })
    expect(waitHours).toHaveAttribute('min', '0.5')
    expect(waitHours).toHaveAttribute('step', '0.5')
    await user.clear(waitHours)
    await user.type(waitHours, '0.5')
    await user.click(screen.getByRole('button', { name: 'Save automation' }))

    await vi.waitFor(() => {
      expect(api.put).toHaveBeenCalledWith(
        '/api/marketing/automations/registration_no_first_call',
        expect.objectContaining({
          trigger_config: expect.objectContaining({
            registration_wait_hours: 0.5,
            max_sends_per_user: 1,
            repeat_interval_days: 2,
          }),
        })
      )
    })
  })

  test('saves referral activation eligibility settings', async () => {
    const user = userEvent.setup()
    renderMarketingPage({
      automations: [
        {
          id: 8,
          scene: 'affiliate_program_activation',
          enabled: false,
          apply_existing: false,
          baseline_ready: true,
          trigger_config: JSON.stringify({
            active_within_days: 30,
            min_request_count: 10,
            min_topup_count: 1,
            max_sends_per_user: 1,
            repeat_interval_days: 30,
          }),
          localized_content: JSON.stringify({
            'zh-CN': { subject: 'Referral', body: 'Earn commission' },
            en: { subject: 'Referral', body: 'Earn commission' },
          }),
          updated_time: 1,
        },
      ],
    })

    await screen.findByText('Marketing launch')
    await user.click(screen.getByRole('tab', { name: 'Automations' }))
    await user.click(
      await screen.findByRole('button', { name: 'Configure automation' })
    )
    expect(
      screen.getByText(
        'This automation only runs while the referral commission program is enabled.'
      )
    ).toBeInTheDocument()
    const activeDays = screen.getByRole('spinbutton', {
      name: 'Active API use within (days)',
    })
    await user.clear(activeDays)
    await user.type(activeDays, '14')
    await user.click(screen.getByRole('button', { name: 'Save automation' }))

    await vi.waitFor(() => {
      expect(api.put).toHaveBeenCalledWith(
        '/api/marketing/automations/affiliate_program_activation',
        expect.objectContaining({
          trigger_config: expect.objectContaining({
            active_within_days: 14,
            min_request_count: 10,
            min_topup_count: 1,
            max_sends_per_user: 1,
          }),
        })
      )
    })
  })
})

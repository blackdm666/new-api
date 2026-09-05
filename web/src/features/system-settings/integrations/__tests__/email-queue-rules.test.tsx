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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { EmailQueueRulesCard, type EmailQueueRules } from '../email-queue-rules'

const rules: EmailQueueRules = {
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
}

describe('email queue rule editor', () => {
  afterEach(() => vi.restoreAllMocks())

  test('blocks a per-minute limit that exceeds the daily cap', () => {
    render(<EmailQueueRulesCard rules={rules} />)

    fireEvent.change(
      screen.getByRole('spinbutton', { name: 'Marketing daily limit' }),
      { target: { value: '10' } }
    )
    fireEvent.change(
      screen.getByRole('spinbutton', { name: 'Marketing per-minute limit' }),
      { target: { value: '20' } }
    )

    expect(
      screen.getByText('Per-minute limit cannot exceed the daily limit')
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Save queue rules' })
    ).toBeDisabled()
  })

  test('persists the complete rule set through the protected option API', async () => {
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    const onSaved = vi.fn()
    render(<EmailQueueRulesCard rules={rules} onSaved={onSaved} />)

    fireEvent.change(
      screen.getByRole('spinbutton', { name: 'Marketing daily limit' }),
      { target: { value: '750' } }
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save queue rules' }))

    await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
    expect(put).toHaveBeenCalledWith('/api/option/', {
      key: 'EmailDeliveryRules',
      value: JSON.stringify({ ...rules, marketing_daily_limit: 750 }),
    })
    expect(onSaved).toHaveBeenCalledTimes(1)
  })

  test('keeps unsaved edits when queue statistics refresh', () => {
    const rendered = render(<EmailQueueRulesCard rules={rules} />)
    const dailyLimit = screen.getByRole('spinbutton', {
      name: 'Marketing daily limit',
    })

    fireEvent.change(dailyLimit, { target: { value: '750' } })
    rendered.rerender(<EmailQueueRulesCard rules={{ ...rules }} />)

    expect(dailyLimit).toHaveValue(750)
  })
})

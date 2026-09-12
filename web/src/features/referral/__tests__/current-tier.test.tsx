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
import { cleanup, render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, test } from 'vitest'

import { CurrentTierBadge } from '../index'
import type { AffiliateSummary } from '../types'

afterEach(cleanup)

async function renderTier(tier: string | undefined, rate = 500) {
  const i18n = createInstance()
  await i18n.use(initReactI18next).init({
    lng: 'zh',
    resources: {
      zh: {
        translation: {
          'Current tier': '当前等级',
          'Junior promoter': '初级推广',
          'Advanced promoter': '高级推广',
          'Gold promoter': '金牌推广',
        },
      },
    },
  })
  const summary: AffiliateSummary | undefined =
    tier === undefined
      ? undefined
      : {
          auto_approve: true,
          available_quota: 0,
          available_cents: 0,
          total_approved_quota: 0,
          pending_commission_cents: 0,
          approved_commission_cents: 0,
          total_topup_cents: 27000,
          invite_count: 36,
          effective_invitee_count: 3,
          commission_record_count: 3,
          rate_basis_points: rate,
          default_rate_basis_points: 500,
          group_rates: { default: 500, 高级推广: 1000, 金牌推广: 1500 },
          tier_name: tier,
          upgrade_eligible: true,
          next_tier_name: '高级推广',
          next_tier_rate_basis_points: 1000,
          upgrade_threshold: 50,
          upgrade_progress: 3,
          upgrade_progress_ratio: 0.06,
          upgrade_top_up_amount_threshold_cents: 200000,
          upgrade_top_up_amount_progress_cents: 27000,
          upgrade_top_up_amount_progress_ratio: 0.135,
        }
  return render(
    <I18nextProvider i18n={i18n}>
      <CurrentTierBadge summary={summary} />
    </I18nextProvider>
  )
}

describe('current referral tier', () => {
  test.each([
    ['初级推广', 500, '初级推广 · 5%'],
    ['高级推广', 1000, '高级推广 · 10%'],
    ['金牌推广', 1500, '金牌推广 · 15%'],
    ['default', 500, '初级推广 · 5%'],
  ])(
    'shows only the actual %s tier as non-interactive text',
    async (tier, rate, label) => {
      await renderTier(tier, rate)
      expect(screen.getByText(label)).toBeVisible()
      expect(
        screen.getAllByText(/^(初级推广|高级推广|金牌推广) ·/)
      ).toHaveLength(1)
      expect(screen.queryAllByRole('button')).toHaveLength(0)
    }
  )

  test.each([
    [750, '高级推广 · 7.5%'],
    [0, '高级推广 · 0%'],
  ])(
    'uses actual backend rate %s without substituting the standard tier rate',
    async (rate, label) => {
      await renderTier('高级推广', rate)
      expect(screen.getByText(label)).toBeVisible()
      expect(screen.queryByText('高级推广 · 10%')).not.toBeInTheDocument()
    }
  )

  test('does not invent a junior tier before summary data arrives', async () => {
    await renderTier(undefined)
    expect(
      screen.queryByText(/^(初级推广|高级推广|金牌推广) ·/)
    ).not.toBeInTheDocument()
  })
})

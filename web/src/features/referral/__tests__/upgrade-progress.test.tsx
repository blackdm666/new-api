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
import type { ReactNode } from 'react'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, test } from 'vitest'

import { SummaryGrid, UpgradeProgress } from '../index'
import { upgradeProgressPercent } from '../lib'
import type { AffiliateSummary } from '../types'

afterEach(cleanup)

function makeSummary(overrides: Partial<AffiliateSummary>): AffiliateSummary {
  return {
    auto_approve: true,
    available_quota: 0,
    available_cents: 0,
    total_approved_quota: 0,
    pending_commission_cents: 0,
    approved_commission_cents: 0,
    invite_count: 36,
    effective_invitee_count: 3,
    commission_record_count: 3,
    invitee_top_up_limit: 10,
    rate_basis_points: 500,
    default_rate_basis_points: 500,
    group_rates: { default: 500, 高级推广: 1000, 金牌推广: 1500 },
    tier_name: '初级推广',
    upgrade_eligible: true,
    highest_tier_reached: false,
    next_tier_name: '高级推广',
    next_tier_rate_basis_points: 1000,
    upgrade_progress_ratio: 0.426,
    ...overrides,
  }
}

async function renderWithI18n(node: ReactNode) {
  const i18n = createInstance()
  await i18n.use(initReactI18next).init({
    lng: 'en',
    resources: { en: { translation: {} } },
  })
  return render(<I18nextProvider i18n={i18n}>{node}</I18nextProvider>)
}

function statValue(label: string): string | null | undefined {
  return screen.getByText(label).nextElementSibling?.textContent
}

describe('referral upgrade progress', () => {
  test('summary shows upgrade progress percent instead of invitee top-up total', async () => {
    // Fields an older server might still send must never be rendered.
    const summary = {
      ...makeSummary({}),
      total_topup_cents: 85_200,
      upgrade_top_up_amount_progress_cents: 85_200,
    } as AffiliateSummary

    await renderWithI18n(<SummaryGrid summary={summary} loading={false} />)

    expect(statValue('Upgrade progress')).toBe('42%')
    expect(screen.queryByText('Invitee top-up amount')).toBeNull()
    expect(document.body.textContent).not.toMatch(/852/)
  })

  test('summary shows highest tier reached for a top-tier promoter', async () => {
    const summary = makeSummary({
      tier_name: '金牌推广',
      upgrade_eligible: false,
      highest_tier_reached: true,
      next_tier_name: '',
      upgrade_progress_ratio: 1,
    })

    await renderWithI18n(<SummaryGrid summary={summary} loading={false} />)

    expect(statValue('Upgrade progress')).toBe('Highest tier reached')
  })

  test('upgrade card shows one combined progress bar without amounts', async () => {
    await renderWithI18n(<UpgradeProgress summary={makeSummary({})} />)

    expect(screen.getByRole('progressbar')).toBeInTheDocument()
    expect(screen.getAllByRole('progressbar')).toHaveLength(1)
    expect(screen.getByText('42%')).toBeVisible()
    expect(screen.queryByText('Effective top-up amount')).toBeNull()
    expect(document.body.textContent).not.toMatch(/¥|\$/)
  })

  test('upgrade card explains the highest tier instead of a progress bar', async () => {
    const summary = makeSummary({
      tier_name: '金牌推广',
      rate_basis_points: 1500,
      upgrade_eligible: false,
      highest_tier_reached: true,
      next_tier_name: '',
      upgrade_progress_ratio: 1,
    })

    await renderWithI18n(<UpgradeProgress summary={summary} />)

    expect(screen.getByText('Highest tier reached')).toBeVisible()
    expect(screen.queryByRole('progressbar')).toBeNull()
  })

  test.each([
    [undefined, 0],
    [Number.NaN, 0],
    [-0.2, 0],
    [0.29, 29],
    [0.999, 99],
    [1, 100],
    [3.5, 100],
  ])('percent for ratio %s is %s', (ratio, expected) => {
    expect(upgradeProgressPercent(ratio)).toBe(expected)
  })
})

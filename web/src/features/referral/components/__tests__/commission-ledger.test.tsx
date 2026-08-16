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
import assert from 'node:assert/strict'

import { describe, test } from 'vitest'

import {
  AFFILIATE_COMMISSION_STATUS,
  type AffiliateCommission,
} from '../../types'

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { CommissionLedgerRow } = await import('../../commission-ledger-admin')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

function commissionFixture(
  id: number,
  tierName: string,
  status: AffiliateCommission['status']
): AffiliateCommission {
  return {
    id,
    inviter_id: id,
    invitee_id: id + 100,
    topup_id: id + 200,
    trade_no: `AFFILIATE-${id}`,
    topup_amount_cents: 10_000,
    rate_basis_points: id * 500,
    inviter_group: tierName,
    tier_name: tierName,
    commission_cents: id * 500,
    commission_quota: id * 500_000,
    status,
    created_time: 1_786_700_800,
    updated_time: 1_786_704_400,
    inviter_username: `promoter_${id}`,
    invitee_username: `invitee_${id}`,
  }
}

describe('affiliate commission ledger', () => {
  test('shows durable status history and distinct tier badge colors without review actions', async () => {
    const items = [
      commissionFixture(1, '初级推广', AFFILIATE_COMMISSION_STATUS.PENDING),
      commissionFixture(2, '高级推广', AFFILIATE_COMMISSION_STATUS.APPROVED),
      commissionFixture(3, '金牌推广', AFFILIATE_COMMISSION_STATUS.REJECTED),
    ]
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    await act(async () => {
      root.render(
        <I18nextProvider i18n={i18n}>
          <table>
            <tbody>
              {items.map((item) => (
                <CommissionLedgerRow key={item.id} item={item} />
              ))}
            </tbody>
          </table>
        </I18nextProvider>
      )
    })

    const text = container.textContent ?? ''
    assert.match(text, /Pending review/)
    assert.match(text, /Approved/)
    assert.match(text, /Rejected/)
    assert.equal(container.querySelectorAll('button').length, 0)

    const badges = [...container.querySelectorAll('[data-slot="badge"]')]
    assert.ok(badges.some((badge) => badge.className.includes('sky-500')))
    assert.ok(badges.some((badge) => badge.className.includes('violet-500')))
    assert.ok(badges.some((badge) => badge.className.includes('amber-500')))

    await act(async () => root.unmount())
    container.remove()
  })
})

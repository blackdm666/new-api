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

import type { AffiliateTransfer } from '../../types'

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { TransferRiskBadge, TransferRiskEvidence } =
  await import('../../transfer-review-admin')
const { affiliateRiskLevel } = await import('../../lib')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

const pendingTransfer: AffiliateTransfer = {
  id: 31,
  user_id: 7340,
  username: 'rainc',
  request_id: 'transfer-review',
  amount_cents: 495,
  amount_quota: 2_475_000,
  balance_cents_before: 495,
  balance_cents_after: 0,
  quota_before: 0,
  quota_after: 0,
  status: 1,
  created_time: 1_792_900_000,
}

async function renderInto(node: React.ReactNode) {
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)
  await act(async () => {
    root.render(<I18nextProvider i18n={i18n}>{node}</I18nextProvider>)
  })
  return {
    text: container.textContent ?? '',
    cleanup: async () => {
      await act(async () => root.unmount())
      container.remove()
    },
  }
}

describe('balance transfer review', () => {
  test('maps the agreed score thresholds to risk levels', () => {
    assert.equal(affiliateRiskLevel(5), 'high')
    assert.equal(affiliateRiskLevel(4), 'medium')
    assert.equal(affiliateRiskLevel(3), 'medium')
    assert.equal(affiliateRiskLevel(2), 'low')
  })

  test('shows a scoring placeholder until the risk has been evaluated', async () => {
    const rendered = await renderInto(
      <TransferRiskBadge item={pendingTransfer} />
    )
    assert.match(rendered.text, /Scoring/)
    await rendered.cleanup()
  })

  test('shows the score, invitee and every signal behind it', async () => {
    const scored: AffiliateTransfer = {
      ...pendingTransfer,
      risk_score: 10,
      risk_evaluated_time: 1_792_900_010,
      risk: {
        invitee_count: 1,
        single_invitee: true,
        invitee_cluster_count: 0,
        suspicious_commission_cents: 495,
        pairs: [
          {
            invitee_id: 7343,
            invitee_username: 'esun',
            score: 9,
            signals: [
              'shared_ip',
              'same_user_agent',
              'registered_within_day',
              'similar_name',
            ],
            shared_ip: '101.68.113.42',
            registration_gap_seconds: 360,
            commission_cents: 495,
          },
        ],
      },
    }
    const badge = await renderInto(<TransferRiskBadge item={scored} />)
    assert.match(badge.text, /10High risk/)
    await badge.cleanup()

    const evidence = await renderInto(<TransferRiskEvidence item={scored} />)
    assert.match(evidence.text, /Only one paying invitee/)
    assert.match(evidence.text, /esun#7343\+9/)
    assert.match(evidence.text, /Shared rare IP 101\.68\.113\.42/)
    assert.match(evidence.text, /Same browser/)
    assert.match(evidence.text, /Registered 6 min apart/)
    assert.match(evidence.text, /Similar username or email/)
    await evidence.cleanup()
  })
})

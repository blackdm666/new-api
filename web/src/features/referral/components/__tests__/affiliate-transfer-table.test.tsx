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

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { AffiliateTransferTable } = await import('../../transfers-admin')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

describe('affiliate transfer audit table', () => {
  test('shows the user, balance snapshots, and idempotency request ID', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    await act(async () => {
      root.render(
        <I18nextProvider i18n={i18n}>
          <AffiliateTransferTable
            loading={false}
            items={[
              {
                id: 7,
                user_id: 3,
                username: 'invoice_demo',
                display_name: 'Demo Promoter',
                request_id: 'transfer-request-2026',
                amount_cents: 100,
                amount_quota: 500_000,
                balance_cents_before: 1_000,
                balance_cents_after: 900,
                quota_before: 1_000_000,
                quota_after: 1_500_000,
                created_time: 1_786_700_800,
              },
            ]}
          />
        </I18nextProvider>
      )
    })

    const text = container.textContent ?? ''
    assert.match(text, /invoice_demoUID 3/)
    assert.doesNotMatch(text, /Demo Promoter/)
    assert.match(text, /transfer-request-2026/)
    assert.match(text, /→/)

    await act(async () => root.unmount())
    container.remove()
  })

  test('shows an explicit empty state', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    await act(async () => {
      root.render(
        <I18nextProvider i18n={i18n}>
          <AffiliateTransferTable loading={false} items={[]} />
        </I18nextProvider>
      )
    })
    assert.match(container.textContent ?? '', /No balance transfer records/)
    await act(async () => root.unmount())
    container.remove()
  })
})

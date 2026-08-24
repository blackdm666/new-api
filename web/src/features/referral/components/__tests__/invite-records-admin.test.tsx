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

import type { AdminAffiliateInviteRecord } from '../../types'

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { AdminInviteRecordRow } = await import('../../invite-records-admin')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

describe('admin invitation records', () => {
  test('shows a registered invitee even when no commission exists', async () => {
    const item: AdminAffiliateInviteRecord = {
      inviter_id: 42,
      inviter_username: 'promoter_42',
      inviter_display_name: 'Promoter Name',
      inviter_remark: 'priority promoter',
      invitee_id: 84,
      invitee_username: 'invitee_84',
      invitee_display_name: 'Invited Name',
      created_at: 1_786_700_000,
      is_new: false,
      topup_count: 0,
      topup_amount_cents: 0,
      commission_cents: 0,
      last_topup_time: 0,
    }
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    let selectedUserId = 0
    await act(async () => {
      root.render(
        <I18nextProvider i18n={i18n}>
          <table>
            <tbody>
              <AdminInviteRecordRow
                item={item}
                onInviterClick={(userId) => {
                  selectedUserId = userId
                }}
              />
            </tbody>
          </table>
        </I18nextProvider>
      )
    })

    const text = container.textContent ?? ''
    assert.match(text, /promoter_42Remark: priority promoterUID 42/)
    assert.match(text, /invitee_84UID 84/)
    assert.doesNotMatch(text, /Promoter Name/)
    assert.doesNotMatch(text, /Invited Name/)
    const cells = container.querySelectorAll('td')
    assert.equal(cells[3]?.textContent, '0')
    assert.equal(cells[6]?.textContent, '-')
    const inviterButton = container.querySelector('button')
    assert.equal(
      inviterButton?.getAttribute('aria-label'),
      'User Information: promoter_42'
    )
    await act(async () => inviterButton?.click())
    assert.equal(selectedUserId, 42)

    await act(async () => root.unmount())
    container.remove()
  })
})

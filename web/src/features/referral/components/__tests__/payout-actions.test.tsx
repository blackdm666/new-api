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
const { PayoutAdminRow } = await import('../../payouts-admin')
const { AFFILIATE_PAYOUT_STATUS } = await import('../../types')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

const basePayout = {
  id: 9,
  user_id: 3,
  request_id: 'payout-test',
  amount_cents: 10_000,
  amount_quota: 5_000_000,
  payment_method: 'alipay' as const,
  account_name: 'Recipient',
  account: 'recipient@example.com',
  account_last4: '.com',
  status: AFFILIATE_PAYOUT_STATUS.APPROVED,
  eligible_settlement_time: 1_786_291_200,
  created_time: 1_786_291_200,
  updated_time: 1_786_291_200,
  username: 'recipient-user',
  display_name: 'Recipient note',
}

function findButton(container: HTMLElement, label: string): HTMLButtonElement {
  const button = [...container.querySelectorAll('button')].find((item) =>
    item.textContent?.includes(label)
  )
  assert.ok(button instanceof HTMLButtonElement)
  return button
}

function findMenuItem(label: string): HTMLElement {
  const item = [
    ...document.body.querySelectorAll<HTMLElement>('[role="menuitem"]'),
  ].find((element) => element.textContent?.includes(label))
  assert.ok(item)
  return item
}

async function openPayoutMenu(container: HTMLElement) {
  await act(async () => {
    findButton(container, 'Settle payout').click()
    await Promise.resolve()
  })
}

describe('administrator payout actions', () => {
  test('groups manual and Alipay actions in one compact settlement menu', async () => {
    const actions: string[] = []
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    await act(async () => {
      root.render(
        <I18nextProvider i18n={i18n}>
          <table>
            <tbody>
              <PayoutAdminRow
                item={basePayout}
                directPayoutAvailable
                settlementOpen
                refreshing={false}
                onAction={(action) => actions.push(action)}
                onRefresh={() => {}}
              />
            </tbody>
          </table>
        </I18nextProvider>
      )
    })

    assert.equal(container.querySelectorAll('button').length, 1)
    assert.match(container.textContent ?? '', /recipient-userUID 3/)
    assert.doesNotMatch(container.textContent ?? '', /Recipient note/)
    assert.match(
      findButton(container, 'Settle payout').textContent ?? '',
      /Settle payout/
    )

    await openPayoutMenu(container)
    await act(async () => findMenuItem('Manual payout').click())
    await openPayoutMenu(container)
    await act(async () => findMenuItem('Alipay direct payout').click())
    assert.deepEqual(actions, ['manual', 'alipay'])
    assert.doesNotMatch(container.textContent ?? '', /Payment reference/)
    await act(async () => root.unmount())
    container.remove()
  })

  test('keeps manual payout available when direct credentials are unavailable', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    await act(async () => {
      root.render(
        <I18nextProvider i18n={i18n}>
          <table>
            <tbody>
              <PayoutAdminRow
                item={basePayout}
                directPayoutAvailable={false}
                settlementOpen
                refreshing={false}
                onAction={() => {}}
                onRefresh={() => {}}
              />
            </tbody>
          </table>
        </I18nextProvider>
      )
    })

    assert.equal(findButton(container, 'Settle payout').disabled, false)
    await openPayoutMenu(container)
    assert.equal(
      findMenuItem('Manual payout').getAttribute('aria-disabled'),
      null
    )
    assert.equal(
      findMenuItem('Alipay direct payout').getAttribute('aria-disabled'),
      'true'
    )
    await act(async () => root.unmount())
    container.remove()
  })

  test('stacks provider failures below the status and keeps payout actions on one line', async () => {
    const providerError =
      'The payout provider rejected this transfer with a deliberately long diagnostic message'
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    await act(async () => {
      root.render(
        <I18nextProvider i18n={i18n}>
          <table>
            <tbody>
              <PayoutAdminRow
                item={{
                  ...basePayout,
                  provider_error_code: 'PROVIDER_REJECTED',
                  provider_error_message: providerError,
                }}
                directPayoutAvailable
                settlementOpen
                refreshing={false}
                onAction={() => {}}
                onRefresh={() => {}}
              />
            </tbody>
          </table>
        </I18nextProvider>
      )
    })

    const errorText = [...container.querySelectorAll('span')].find(
      (element) => element.textContent === providerError
    )
    assert.ok(errorText)
    assert.equal(errorText.parentElement?.classList.contains('flex-col'), true)

    const settlementButton = findButton(container, 'Settle payout')
    assert.equal(
      settlementButton.closest('td')?.classList.contains('min-w-[150px]'),
      true
    )
    assert.equal(
      settlementButton.closest('td')?.classList.contains('align-middle'),
      true
    )
    assert.equal(
      errorText.closest('td')?.classList.contains('align-middle'),
      true
    )
    assert.equal(
      settlementButton.closest('td')?.classList.contains('whitespace-nowrap'),
      true
    )

    await act(async () => root.unmount())
    container.remove()
  })

  test('only permits status refresh while an Alipay payout is processing', async () => {
    let refreshes = 0
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    await act(async () => {
      root.render(
        <I18nextProvider i18n={i18n}>
          <table>
            <tbody>
              <PayoutAdminRow
                item={{
                  ...basePayout,
                  status: AFFILIATE_PAYOUT_STATUS.PROCESSING,
                  disbursement_mode: 'alipay_direct',
                }}
                directPayoutAvailable
                settlementOpen
                refreshing={false}
                onAction={() => {}}
                onRefresh={() => {
                  refreshes += 1
                }}
              />
            </tbody>
          </table>
        </I18nextProvider>
      )
    })

    await act(async () =>
      findButton(container, 'Refresh payout status').click()
    )
    assert.equal(refreshes, 1)
    assert.equal(
      [...container.querySelectorAll('button')].some((button) =>
        button.textContent?.includes('Settle payout')
      ),
      false
    )
    await act(async () => root.unmount())
    container.remove()
  })

  test('keeps rejection reasons user-facing and localizes signature diagnostics for administrators', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    await act(async () => {
      root.render(
        <I18nextProvider i18n={i18n}>
          <table>
            <tbody>
              <PayoutAdminRow
                item={{
                  ...basePayout,
                  status: AFFILIATE_PAYOUT_STATUS.REJECTED,
                  reject_reason: 'Reason intended for the user',
                  provider_error_code: 'RESPONSE_SIGNATURE_VERIFICATION_FAILED',
                  provider_error_message:
                    'response signature verification failed',
                }}
                directPayoutAvailable
                settlementOpen
                refreshing={false}
                onAction={() => {}}
                onRefresh={() => {}}
              />
            </tbody>
          </table>
        </I18nextProvider>
      )
    })

    assert.doesNotMatch(
      container.textContent ?? '',
      /Reason intended for the user/
    )
    assert.match(
      container.textContent ?? '',
      /Alipay response verification failed/
    )
    await act(async () => root.unmount())
    container.remove()
  })
})

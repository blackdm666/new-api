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
import { after, describe, test } from 'node:test'

import { Window } from 'happy-dom'
import type React from 'react'

const domWindow = new Window()
for (const key of [
  'window',
  'document',
  'navigator',
  'HTMLElement',
  'HTMLButtonElement',
  'SVGElement',
  'Node',
  'Element',
  'Event',
  'MouseEvent',
] as const) {
  Object.defineProperty(globalThis, key, {
    configurable: true,
    value: domWindow[key],
  })
}

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { CommissionActions } = await import('../../commission-actions')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

type RenderedActions = {
  container: HTMLDivElement
  root: ReturnType<typeof createRoot>
}

async function renderActions(
  props: React.ComponentProps<typeof CommissionActions>
): Promise<RenderedActions> {
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)
  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <CommissionActions {...props} />
      </I18nextProvider>
    )
  })
  return { container, root }
}

function findButton(container: HTMLElement, label: string): HTMLButtonElement {
  const button = [...container.querySelectorAll('button')].find((item) =>
    item.textContent?.includes(label)
  )
  assert.ok(button instanceof HTMLButtonElement)
  return button
}

async function unmountActions(rendered: RenderedActions) {
  await act(async () => rendered.root.unmount())
  rendered.container.remove()
}

describe('commission actions', () => {
  after(() => domWindow.close())

  test('opens the matching confirmation flow from each action', async () => {
    let transferClicks = 0
    let payoutClicks = 0
    const rendered = await renderActions({
      availableCents: 12_850,
      minimumPayoutCents: 10_000,
      settlementDay: 10,
      loading: false,
      onTransfer: () => {
        transferClicks += 1
      },
      onPayout: () => {
        payoutClicks += 1
      },
    })

    await act(async () => {
      findButton(rendered.container, 'Transfer to API balance').click()
      findButton(rendered.container, 'Apply for cash payout').click()
    })

    assert.equal(transferClicks, 1)
    assert.equal(payoutClicks, 1)
    assert.match(
      rendered.container.textContent ?? '',
      /Instant credit · no referral commission/
    )
    assert.match(rendered.container.textContent ?? '', /day 10 each month/)
    await unmountActions(rendered)
  })

  test('disables unavailable transfer and cash payout actions', async () => {
    const rendered = await renderActions({
      availableCents: 99,
      minimumPayoutCents: 10_000,
      settlementDay: 10,
      loading: false,
      onTransfer: () => {},
      onPayout: () => {},
    })

    assert.equal(
      findButton(rendered.container, 'Transfer to API balance').disabled,
      true
    )
    assert.equal(
      findButton(rendered.container, 'Apply for cash payout').disabled,
      true
    )
    await unmountActions(rendered)
  })
})

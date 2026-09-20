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

const { act, useState } = await import('react')
const { createRoot } = await import('react-dom/client')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { QueryClient, QueryClientProvider } =
  await import('@tanstack/react-query')
const { UserInviterSelector } = await import('../user-inviter-selector')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        Disabled: 'Disabled',
        Loading: 'Loading',
        'No Inviter': 'No Inviter',
        'No matching results': 'No matching results',
        'Search by user ID, username, display name, or email':
          'Search by user ID, username, display name, or email',
      },
    },
  },
})

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

function Harness(props: { queryClient: InstanceType<typeof QueryClient> }) {
  const [value, setValue] = useState(7)

  return (
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={props.queryClient}>
        <UserInviterSelector
          targetUserId={9}
          value={value}
          onValueChange={setValue}
        />
        <output data-testid='selected-inviter'>{value}</output>
      </QueryClientProvider>
    </I18nextProvider>
  )
}

function commandItem(label: string): HTMLElement {
  const item = [
    ...document.querySelectorAll<HTMLElement>('[data-slot="command-item"]'),
  ].find((candidate) => candidate.textContent?.includes(label))
  assert.ok(item)
  return item
}

describe('user inviter selector', () => {
  test('shows the saved inviter and allows selecting or clearing it', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const response = {
      success: true,
      data: [
        {
          id: 7,
          username: 'original-inviter',
          display_name: 'Original Inviter',
          status: 1,
        },
        {
          id: 8,
          username: 'replacement-inviter',
          display_name: 'Replacement Inviter',
          status: 1,
        },
      ],
    }
    queryClient.setQueryData(['user-inviter-options', 9, 7, ''], response)
    queryClient.setQueryData(['user-inviter-options', 9, 8, ''], response)
    queryClient.setQueryData(
      ['user-inviter-options', 9, undefined, ''],
      response
    )

    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    await act(async () => root.render(<Harness queryClient={queryClient} />))

    const trigger = container.querySelector<HTMLButtonElement>(
      'button[role="combobox"]'
    )
    assert.ok(trigger)
    assert.match(trigger.textContent || '', /Original Inviter/)
    assert.equal(trigger.getAttribute('aria-expanded'), 'false')

    await act(async () => trigger.click())
    assert.equal(trigger.getAttribute('aria-expanded'), 'true')
    await act(async () => commandItem('Replacement Inviter').click())
    assert.equal(
      container.querySelector('[data-testid="selected-inviter"]')?.textContent,
      '8'
    )
    assert.equal(trigger.getAttribute('aria-expanded'), 'false')

    await act(async () => trigger.click())
    await act(async () => commandItem('No Inviter').click())
    assert.equal(
      container.querySelector('[data-testid="selected-inviter"]')?.textContent,
      '0'
    )

    await act(async () => root.unmount())
    container.remove()
  })
})

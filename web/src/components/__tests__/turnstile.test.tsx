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

import { beforeEach, describe, test } from 'vitest'

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const { Turnstile } = await import('../turnstile')
const { resolveTurnstileEndpoint } = await import('../turnstile-utils')

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

describe('Turnstile compatibility component', () => {
  beforeEach(() => {
    document.head.innerHTML = ''
    document.body.innerHTML = ''
    delete window.Captcha88
    delete window.turnstile
  })

  test('routes self-hosted verification through the optional development proxy', () => {
    assert.equal(
      resolveTurnstileEndpoint('https://verify.88api.ai/', true),
      '/__turnstile'
    )
    assert.equal(
      resolveTurnstileEndpoint('https://verify.88api.ai/', false),
      'https://verify.88api.ai'
    )
  })

  test('uses Captcha88 when the configured site key is a service URL', async () => {
    let renderOptions:
      | Parameters<NonNullable<typeof window.Captcha88>['render']>[0]
      | undefined
    let verifiedToken = ''
    window.Captcha88 = {
      render: (options) => {
        renderOptions = options
      },
    }

    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    await act(async () => {
      root.render(
        <Turnstile
          siteKey='https://verify.88api.ai/'
          onVerify={(token) => {
            verifiedToken = token
          }}
        />
      )
    })

    assert.equal(
      renderOptions?.endpoint,
      resolveTurnstileEndpoint('https://verify.88api.ai/', import.meta.env.DEV)
    )
    assert.equal(renderOptions?.act, 'register')
    renderOptions?.onToken?.('one-use-token')
    assert.equal(verifiedToken, 'one-use-token')

    await act(async () => root.unmount())
  })

  test('keeps standard Cloudflare Turnstile available for ordinary site keys', async () => {
    let renderedSiteKey = ''
    window.turnstile = {
      render: (_element, options) => {
        renderedSiteKey = String(options.sitekey)
      },
    }

    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    await act(async () => {
      root.render(
        <Turnstile siteKey='cloudflare-site-key' onVerify={() => {}} />
      )
    })

    assert.equal(renderedSiteKey, 'cloudflare-site-key')
    await act(async () => root.unmount())
  })

  test('loads the Captcha88 widget script from the configured service URL', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    await act(async () => {
      root.render(
        <Turnstile siteKey='https://verify.88api.ai/' onVerify={() => {}} />
      )
    })

    const script = document.querySelector<HTMLScriptElement>('#c88-widget')
    assert.equal(script?.src, 'https://verify.88api.ai/widget.js')
    await act(async () => root.unmount())
  })
})

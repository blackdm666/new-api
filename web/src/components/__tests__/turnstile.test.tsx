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
const {
  getTurnstileClientConfig,
  isTurnstileClientConfigured,
  resolveTurnstileWidgetEndpoint,
} = await import('../turnstile-utils')

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

  test('builds explicit client configuration from system status', () => {
    const config = getTurnstileClientConfig({
      turnstile_provider: 'custom',
      turnstile_site_key: 'optional-site-key',
      turnstile_widget_script_url: 'https://captcha.example/widget.js',
      turnstile_widget_endpoint: 'https://captcha.example',
      turnstile_action: 'login',
    })

    assert.deepEqual(config, {
      provider: 'custom',
      siteKey: 'optional-site-key',
      widgetScriptURL: 'https://captcha.example/widget.js',
      widgetEndpoint: 'https://captcha.example',
      action: 'login',
    })
    assert.equal(isTurnstileClientConfigured(config), true)
  })

  test('uses the opt-in development proxy without changing saved configuration', () => {
    assert.equal(
      resolveTurnstileWidgetEndpoint('https://captcha.example/api', true),
      '/__captcha'
    )
    assert.equal(
      resolveTurnstileWidgetEndpoint('https://captcha.example/api', false),
      'https://captcha.example/api'
    )
  })

  test('uses Captcha88 only when the custom provider is selected', async () => {
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
          provider='custom'
          siteKey='custom-site-key'
          widgetScriptURL='https://captcha.example/widget.js'
          widgetEndpoint='https://captcha.example/api'
          action='login'
          onVerify={(token) => {
            verifiedToken = token
          }}
        />
      )
    })

    assert.equal(renderOptions?.endpoint, 'https://captcha.example/api')
    assert.equal(renderOptions?.act, 'login')
    assert.equal(renderOptions?.siteKey, 'custom-site-key')
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
        <Turnstile
          provider='cloudflare'
          siteKey='cloudflare-site-key'
          widgetScriptURL=''
          widgetEndpoint=''
          action='register'
          onVerify={() => {}}
        />
      )
    })

    assert.equal(renderedSiteKey, 'cloudflare-site-key')
    await act(async () => root.unmount())
  })

  test('loads the custom widget script from the configured URL', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    await act(async () => {
      root.render(
        <Turnstile
          provider='custom'
          siteKey=''
          widgetScriptURL='https://captcha.example/assets/slider.js'
          widgetEndpoint='https://captcha.example/api'
          action='register'
          onVerify={() => {}}
        />
      )
    })

    const script = document.querySelector<HTMLScriptElement>(
      '#custom-turnstile-widget'
    )
    assert.equal(script?.src, 'https://captcha.example/assets/slider.js')
    await act(async () => root.unmount())
  })

  test('keeps one widget instance when callback identities change', async () => {
    let renderCount = 0
    let renderOptions:
      | Parameters<NonNullable<typeof window.Captcha88>['render']>[0]
      | undefined
    const verifiedTokens: string[] = []
    window.Captcha88 = {
      render: (options) => {
        renderCount += 1
        renderOptions = options
      },
    }

    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    await act(async () => {
      root.render(
        <Turnstile
          provider='custom'
          siteKey=''
          widgetScriptURL='https://captcha.example/widget.js'
          widgetEndpoint='https://captcha.example'
          action='register'
          onVerify={() => verifiedTokens.push('first')}
          onExpire={() => {}}
        />
      )
    })
    await act(async () => {
      root.render(
        <Turnstile
          provider='custom'
          siteKey=''
          widgetScriptURL='https://captcha.example/widget.js'
          widgetEndpoint='https://captcha.example'
          action='register'
          onVerify={() => verifiedTokens.push('latest')}
          onExpire={() => {}}
        />
      )
    })

    renderOptions?.onToken?.('one-use-token')
    assert.equal(renderCount, 1)
    assert.deepEqual(verifiedTokens, ['latest'])
    await act(async () => root.unmount())
  })

  test('removes the old Cloudflare widget before rendering new configuration', async () => {
    const renderedSiteKeys: string[] = []
    const removedWidgetIds: Array<string | number> = []
    window.turnstile = {
      render: (_element, options) => {
        renderedSiteKeys.push(String(options.sitekey))
        return `widget-${renderedSiteKeys.length}`
      },
      remove: (widgetId) => removedWidgetIds.push(widgetId),
    }

    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    await act(async () => {
      root.render(
        <Turnstile
          provider='cloudflare'
          siteKey='site-one'
          widgetScriptURL=''
          widgetEndpoint=''
          action='register'
          onVerify={() => {}}
        />
      )
    })
    await act(async () => {
      root.render(
        <Turnstile
          provider='cloudflare'
          siteKey='site-two'
          widgetScriptURL=''
          widgetEndpoint=''
          action='register'
          onVerify={() => {}}
        />
      )
    })

    assert.deepEqual(renderedSiteKeys, ['site-one', 'site-two'])
    assert.deepEqual(removedWidgetIds, ['widget-1'])
    await act(async () => root.unmount())
    assert.deepEqual(removedWidgetIds, ['widget-1', 'widget-2'])
  })
})

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

import { Window as HappyDOMWindow } from 'happy-dom'

import type { GeeTestValidation } from '@/features/auth/types'

const domWindow = new HappyDOMWindow()
const domGlobals = [
  'window',
  'document',
  'navigator',
  'HTMLElement',
  'Node',
  'Element',
  'Event',
  'CustomEvent',
  'MutationObserver',
  'requestAnimationFrame',
  'cancelAnimationFrame',
  'getComputedStyle',
] as const

for (const key of domGlobals) {
  Object.defineProperty(globalThis, key, {
    configurable: true,
    value: domWindow[key],
  })
}

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const i18next = (await import('i18next')).default
const { initReactI18next } = await import('react-i18next')
await i18next.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        'Security verification': 'Security verification',
        'Loading...': 'Loading...',
        'Verification failed': 'Verification failed',
      },
    },
  },
})

type GeeTestCallback = () => void
type GeeTestSDK = NonNullable<typeof window.initGeetest4>
type GeeTestSDKOptions = Parameters<GeeTestSDK>[0]
type GeeTestSDKInstance = Parameters<Parameters<GeeTestSDK>[1]>[0]

const validation: GeeTestValidation = {
  lot_number: 'lot-1',
  captcha_output: 'output',
  pass_token: 'pass',
  gen_time: '123',
}
let successCallback: GeeTestCallback | undefined
let receivedOptions: GeeTestSDKOptions | undefined
let destroyed = false

const fakeInstance: GeeTestSDKInstance = {
  appendTo(target) {
    target.setAttribute('data-geetest-mounted', 'true')
  },
  destroy() {
    destroyed = true
  },
  getValidate: () => validation,
  onReady(callback) {
    callback()
    return this
  },
  onSuccess(callback) {
    successCallback = callback
    return this
  },
  onFail() {
    return this
  },
  onError() {
    return this
  },
  onClose() {
    return this
  },
}

window.initGeetest4 = (options, callback) => {
  receivedOptions = options
  callback(fakeInstance)
}

const { GeeTest } = await import('../geetest')
const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

describe('GeeTest component', () => {
  after(() => {
    domWindow.close()
  })

  test('initializes v4 with the public captcha ID and returns its proof', async () => {
    const proofs: Array<GeeTestValidation | undefined> = []
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)

    await act(async () => {
      root.render(
        <GeeTest
          captchaId='public-register-id'
          onVerify={(proof) => proofs.push(proof)}
        />
      )
    })

    assert.equal(receivedOptions?.captchaId, 'public-register-id')
    assert.equal(receivedOptions?.product, 'float')
    assert.equal(
      container
        .querySelector('[data-geetest-mounted]')
        ?.getAttribute('data-geetest-mounted'),
      'true'
    )

    await act(async () => successCallback?.())
    assert.deepEqual(proofs, [undefined, validation])

    await act(async () => root.unmount())
    assert.equal(destroyed, true)
    container.remove()
  })
})

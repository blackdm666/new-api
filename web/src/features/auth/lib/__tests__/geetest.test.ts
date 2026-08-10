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
import { describe, test } from 'node:test'

import { getGeeTestPublicConfig, getGeeTestQueryParams } from '../geetest'

describe('GeeTest public configuration', () => {
  test('keeps registration and login scenes independent', () => {
    const status = {
      geetest_register_check: true,
      geetest_register_captcha_id: 'register-id',
      geetest_login_check: true,
      geetest_login_captcha_id: 'login-id',
    }

    assert.deepEqual(getGeeTestPublicConfig(status, 'register'), {
      enabled: true,
      captchaId: 'register-id',
    })
    assert.deepEqual(getGeeTestPublicConfig(status, 'login'), {
      enabled: true,
      captchaId: 'login-id',
    })
  })

  test('does not enable a scene without a public captcha id', () => {
    assert.deepEqual(
      getGeeTestPublicConfig({ geetest_register_check: true }, 'register'),
      { enabled: false, captchaId: '' }
    )
  })
})

describe('GeeTest email verification query contract', () => {
  test('maps all four one-time proof fields', () => {
    assert.deepEqual(
      getGeeTestQueryParams({
        lot_number: 'lot',
        captcha_output: 'output',
        pass_token: 'pass',
        gen_time: '123',
      }),
      {
        geetest_lot_number: 'lot',
        geetest_captcha_output: 'output',
        geetest_pass_token: 'pass',
        geetest_gen_time: '123',
      }
    )
  })
})

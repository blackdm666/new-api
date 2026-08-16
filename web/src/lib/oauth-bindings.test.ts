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

import { normalizeOAuthBindings } from './oauth-bindings'

describe('OAuth binding response normalization', () => {
  test('turns non-array payloads into an empty list', () => {
    for (const value of [undefined, null, {}, '[]']) {
      assert.deepEqual(normalizeOAuthBindings(value), [])
    }
  })

  test('keeps an empty backend list as an empty list', () => {
    assert.deepEqual(normalizeOAuthBindings([]), [])
  })

  test('normalizes the native backend binding shape', () => {
    assert.deepEqual(
      normalizeOAuthBindings([
        {
          provider_id: 12,
          provider_name: 'Google',
          provider_slug: 'google',
          provider_icon: 'google',
          provider_user_id: 'google-user-42',
          user_id: 7,
        },
      ]),
      [
        {
          provider_id: 12,
          provider_name: 'Google',
          provider_slug: 'google',
          provider_icon: 'google',
          provider_user_id: 'google-user-42',
        },
      ]
    )
  })

  test('accepts the older proxy-compatible binding shape', () => {
    assert.deepEqual(
      normalizeOAuthBindings([
        {
          provider_id: '12',
          provider_name: 'Google',
          external_id: 'legacy-user-42',
        },
      ]),
      [
        {
          provider_id: 12,
          provider_name: 'Google',
          provider_slug: '',
          provider_icon: '',
          provider_user_id: 'legacy-user-42',
        },
      ]
    )
  })

  test('drops invalid entries without rejecting the valid bindings', () => {
    assert.deepEqual(
      normalizeOAuthBindings([null, {}, { provider_id: 3, external_id: 99 }]),
      [
        {
          provider_id: 3,
          provider_name: '3',
          provider_slug: '',
          provider_icon: '',
          provider_user_id: '99',
        },
      ]
    )
  })
})

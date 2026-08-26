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

import type { User } from '../../types'
import {
  transformFormDataToPayload,
  transformUserToFormDefaults,
  USER_FORM_DEFAULT_VALUES,
} from '../user-form'

const existingUser: User = {
  id: 9,
  username: 'invitee',
  display_name: 'Invitee',
  quota: 100,
  used_quota: 0,
  request_count: 0,
  group: 'default',
  inviter_id: 7,
  status: 1,
  role: 1,
}

describe('user inviter form mapping', () => {
  test('loads and submits the saved inviter for an existing user', () => {
    const defaults = transformUserToFormDefaults(existingUser)

    assert.equal(defaults.inviter_id, 7)
    assert.equal(
      transformFormDataToPayload(defaults, existingUser.id).inviter_id,
      7
    )
  })

  test('submits zero to clear an existing invitation relationship', () => {
    const payload = transformFormDataToPayload(
      { ...transformUserToFormDefaults(existingUser), inviter_id: 0 },
      existingUser.id
    )

    assert.equal(payload.inviter_id, 0)
  })

  test('does not send inviter data while creating a user', () => {
    const payload = transformFormDataToPayload({
      ...USER_FORM_DEFAULT_VALUES,
      username: 'new-user',
      inviter_id: 12,
    })

    assert.equal('inviter_id' in payload, false)
  })
})

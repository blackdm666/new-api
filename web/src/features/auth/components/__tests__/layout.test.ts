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

import { oauthProviderGridClassName } from '../oauth-provider-layout'

describe('OAuth provider button layout', () => {
  test('uses one column by default and two columns from the small breakpoint', () => {
    const classes = new Set(oauthProviderGridClassName.split(' '))

    assert.ok(classes.has('grid'))
    assert.ok(classes.has('grid-cols-1'))
    assert.ok(classes.has('sm:grid-cols-2'))
    assert.ok(!classes.has('grid-cols-2'))
  })
})

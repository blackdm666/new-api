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

import {
  buildMarketingGroupOptions,
  normalizeMarketingGroups,
} from './marketing-groups'

describe('marketing user group selection', () => {
  test('normalizes the saved campaign groups for multi-select state', () => {
    assert.deepEqual(
      normalizeMarketingGroups([' default ', 'vip', '', 'vip']),
      ['default', 'vip']
    )
  })

  test('maps current system groups and preserves removed saved groups', () => {
    assert.deepEqual(
      buildMarketingGroupOptions(
        ['vip', 'default', 'enterprise'],
        ['legacy', 'vip']
      ),
      [
        { value: 'default', label: 'default' },
        { value: 'enterprise', label: 'enterprise' },
        { value: 'legacy', label: 'legacy' },
        { value: 'vip', label: 'vip' },
      ]
    )
  })
})

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
import { describe, expect, test } from 'vitest'

import type { AdvancedCustomConfig } from '../../types'
import { normalizeAdvancedCustomConfig } from '../advanced-custom'

describe('advanced custom Claude thinking compatibility', () => {
  test('normalization drops the deprecated route-level opt-in and keeps the route', () => {
    const config: AdvancedCustomConfig = {
      advanced_routes: [
        {
          incoming_path: '/v1/messages',
          upstream_path: '/v1/messages',
          converter: 'none',
          models: ['claude-opus-5-5'],
          pass_through_body_enabled: true,
          claude_adaptive_thinking_compatibility: true,
        },
      ],
    }

    const normalized = normalizeAdvancedCustomConfig(config)

    expect(normalized.advanced_routes).toEqual([
      {
        incoming_path: '/v1/messages',
        upstream_path: '/v1/messages',
        converter: 'none',
        models: ['claude-opus-5-5'],
        pass_through_body_enabled: true,
      },
    ])
  })
})

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

import { DEFAULT_CONFIG, DEFAULT_PARAMETER_ENABLED } from '../../constants'
import type { Message } from '../../types'
import { buildChatCompletionPayload } from './payload-builder'

const messages: Message[] = [
  {
    key: 'user-1',
    from: 'user',
    versions: [{ id: 'version-1', content: 'test' }],
    status: 'complete',
    createdAt: 1,
  },
]

describe('buildChatCompletionPayload', () => {
  test('omits enabled zero-value penalties', () => {
    const payload = buildChatCompletionPayload(
      messages,
      { ...DEFAULT_CONFIG, model: 'grok-4.3' },
      DEFAULT_PARAMETER_ENABLED
    )

    expect(payload).not.toHaveProperty('frequency_penalty')
    expect(payload).not.toHaveProperty('presence_penalty')
  })

  test('preserves non-zero penalties for compatible models', () => {
    const payload = buildChatCompletionPayload(
      messages,
      {
        ...DEFAULT_CONFIG,
        model: 'gpt-4o',
        frequency_penalty: 0.5,
        presence_penalty: -0.25,
      },
      DEFAULT_PARAMETER_ENABLED
    )

    expect(payload.frequency_penalty).toBe(0.5)
    expect(payload.presence_penalty).toBe(-0.25)
  })
})

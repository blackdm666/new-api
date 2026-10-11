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

import { CHANNEL_TYPE_SUB2API } from '../../constants'
import { channelSchema } from '../../types'
import { CHANNEL_TYPE_ADVANCED_CUSTOM } from '../advanced-custom'
import {
  transformChannelToFormDefaults,
  transformFormDataToUpdatePayload,
} from '../channel-form'

const CLAUDE_MESSAGES_ROUTE = {
  incoming_path: '/v1/messages',
  upstream_path: '/v1/messages',
  converter: 'none',
  models: ['claude-opus-5-5'],
}

function loadChannel(type: number, settings: Record<string, unknown>) {
  return channelSchema.parse({
    id: 1,
    name: 'Claude channel',
    key: '',
    type,
    status: 1,
    created_time: 0,
    test_time: 0,
    response_time: 0,
    balance_updated_time: 0,
    settings: JSON.stringify(settings),
  })
}

function saveSettings(channel: ReturnType<typeof loadChannel>) {
  const payload = transformFormDataToUpdatePayload(
    transformChannelToFormDefaults(channel),
    channel.id
  )
  return JSON.parse(String(payload.settings)) as Record<string, unknown>
}

describe('Claude thinking compatibility channel setting', () => {
  test('loads the deprecated channel opt-in as always and saves only the new mode', () => {
    const channel = loadChannel(14, {
      claude_adaptive_thinking_compatibility: true,
    })

    expect(
      transformChannelToFormDefaults(channel).claude_thinking_compat_mode
    ).toBe('always')
    const saved = saveSettings(channel)
    expect(saved.claude_thinking_compat_mode).toBe('always')
    expect(saved).not.toHaveProperty('claude_adaptive_thinking_compatibility')
  })

  test('loads a deprecated advanced custom route opt-in as the channel always mode and drops the route flag on save', () => {
    const channel = loadChannel(CHANNEL_TYPE_ADVANCED_CUSTOM, {
      advanced_custom: {
        advanced_routes: [
          {
            ...CLAUDE_MESSAGES_ROUTE,
            claude_adaptive_thinking_compatibility: true,
          },
        ],
      },
    })

    expect(
      transformChannelToFormDefaults(channel).claude_thinking_compat_mode
    ).toBe('always')
    const saved = saveSettings(channel)
    expect(saved.claude_thinking_compat_mode).toBe('always')
    const routes = (saved.advanced_custom as { advanced_routes: object[] })
      .advanced_routes
    expect(routes).toHaveLength(1)
    expect(routes[0]).not.toHaveProperty(
      'claude_adaptive_thinking_compatibility'
    )
  })

  test('an explicit mode wins over the deprecated opt-in', () => {
    const channel = loadChannel(CHANNEL_TYPE_SUB2API, {
      claude_thinking_compat_mode: 'off',
      claude_adaptive_thinking_compatibility: true,
    })

    expect(
      transformChannelToFormDefaults(channel).claude_thinking_compat_mode
    ).toBe('off')
    const saved = saveSettings(channel)
    expect(saved.claude_thinking_compat_mode).toBe('off')
    expect(saved).not.toHaveProperty('claude_adaptive_thinking_compatibility')
  })

  test('unset or unknown modes load as auto and are saved explicitly', () => {
    for (const settings of [{}, { claude_thinking_compat_mode: 'sometimes' }]) {
      const channel = loadChannel(14, settings)

      expect(
        transformChannelToFormDefaults(channel).claude_thinking_compat_mode
      ).toBe('auto')
      expect(saveSettings(channel).claude_thinking_compat_mode).toBe('auto')
    }
  })

  test('channel types without native Claude Messages drop both fields on save', () => {
    const saved = saveSettings(
      loadChannel(1, {
        claude_thinking_compat_mode: 'always',
        claude_adaptive_thinking_compatibility: true,
      })
    )

    expect(saved).not.toHaveProperty('claude_thinking_compat_mode')
    expect(saved).not.toHaveProperty('claude_adaptive_thinking_compatibility')
  })
})

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

import {
  CHANNEL_TYPE_OPTIONS,
  CHANNEL_TYPE_XINMENG,
  MODEL_FETCHABLE_TYPES,
} from '../../constants'
import {
  getChannelTypeConfig,
  getChannelTypeSupportedModels,
} from '../channel-type-config'
import { getChannelTypeIcon, getKeyPromptForType } from '../channel-utils'

describe('XinMeng channel', () => {
  test('registers upstream discovery and the current related models', () => {
    expect(
      CHANNEL_TYPE_OPTIONS.find((item) => item.value === CHANNEL_TYPE_XINMENG)
    ).toEqual({ value: CHANNEL_TYPE_XINMENG, label: 'XinMeng' })
    expect(MODEL_FETCHABLE_TYPES.has(CHANNEL_TYPE_XINMENG)).toBe(true)
    expect(getChannelTypeIcon(CHANNEL_TYPE_XINMENG)).toBe('Volcengine')
    expect(getKeyPromptForType(CHANNEL_TYPE_XINMENG)).toBe(
      'Enter XinMeng Bearer API key'
    )
    expect(getChannelTypeConfig(CHANNEL_TYPE_XINMENG)).toMatchObject({
      defaultBaseUrl: 'https://www.jimengvip.online',
      supportedModels: [
        'dvc-seedance-2.5',
        'dvc-seedance-2.0',
        'minimax-h3-768p',
        'doubao-seedance-2-5-720p',
        'doubao-seedance-2-0-720p',
        'doubao-seedance-2-0-fast-720p',
        'seedance-2.0-mini-480p',
        'seedance-2.0-mini-720p',
        'wan3.0-video-720p',
        'wan3.0-video-1080p',
        'kling-3.0-turbo-720p',
        'kling-3.0-turbo-1080p',
        'kling-3.0-turbo-2k',
        'kling-3.0-turbo-4k',
      ],
    })
    expect(getChannelTypeSupportedModels(CHANNEL_TYPE_XINMENG)).toEqual([
      'dvc-seedance-2.5',
      'dvc-seedance-2.0',
      'minimax-h3-768p',
      'doubao-seedance-2-5-720p',
      'doubao-seedance-2-0-720p',
      'doubao-seedance-2-0-fast-720p',
      'seedance-2.0-mini-480p',
      'seedance-2.0-mini-720p',
      'wan3.0-video-720p',
      'wan3.0-video-1080p',
      'kling-3.0-turbo-720p',
      'kling-3.0-turbo-1080p',
      'kling-3.0-turbo-2k',
      'kling-3.0-turbo-4k',
    ])
    expect(getChannelTypeSupportedModels(CHANNEL_TYPE_XINMENG)).not.toContain(
      'kling-3.0-turbo'
    )
  })
})

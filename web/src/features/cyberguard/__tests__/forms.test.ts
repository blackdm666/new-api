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
import type { TFunction } from 'i18next'
import { describe, expect, test } from 'vitest'

import {
  getBanFormSchema,
  getSettingsFormSchema,
  parseIdList,
  settingsFormToPayload,
  settingsToFormValues,
} from '../lib/forms'
import type { CyberGuardSettings } from '../types'

const t = ((key: string) => key) as TFunction

const settings: CyberGuardSettings = {
  mode: 'observe',
  protected_channel_ids: [1, 9, 37],
  sub2api_base_url: 'http://sub2api:8080',
  sub2api_api_key_ids: [1, 6],
  poll_interval_seconds: 30,
  lookback_minutes: 15,
  count_categories: ['cyber', 'bio'],
  nosession_merge_seconds: 60,
  tiers: [
    { window_days: 7, min_units: 2, ban_hours: 72 },
    { window_days: 30, min_units: 3, ban_hours: 0 },
  ],
  ban_group: '您因违反使用规定已被封禁',
  exempt_user_ids: [],
  email_enabled: true,
  notify_root: true,
  has_sub2api_admin_api_key: true,
}

describe('parseIdList', () => {
  test('mixed separators and duplicates yield unique ids in input order', () => {
    expect(parseIdList('9, 1，37 9\n1')).toEqual([9, 1, 37])
  })

  test('empty text yields an empty list', () => {
    expect(parseIdList('  ')).toEqual([])
  })

  test('zero, negative or non-numeric parts reject the whole list', () => {
    expect(parseIdList('1, 0')).toBeNull()
    expect(parseIdList('1, -2')).toBeNull()
    expect(parseIdList('1, abc')).toBeNull()
  })
})

describe('settings payload', () => {
  test('round trip without a new key keeps the stored admin key', () => {
    const payload = settingsFormToPayload(settingsToFormValues(settings))

    expect(payload).not.toHaveProperty('sub2api_admin_api_key')
    expect(payload).toEqual({
      mode: 'observe',
      sub2api_base_url: 'http://sub2api:8080',
      sub2api_api_key_ids: [1, 6],
      protected_channel_ids: [1, 9, 37],
      poll_interval_seconds: 30,
      lookback_minutes: 15,
      nosession_merge_seconds: 60,
      count_categories: ['cyber', 'bio'],
      tiers: settings.tiers,
      ban_group: '您因违反使用规定已被封禁',
      exempt_user_ids: [],
      email_enabled: true,
      notify_root: true,
    })
  })

  test('a typed admin key is trimmed and sent', () => {
    const values = settingsToFormValues(settings)
    values.sub2api_admin_api_key = '  sk-admin-new  '

    expect(settingsFormToPayload(values).sub2api_admin_api_key).toBe(
      'sk-admin-new'
    )
  })
})

describe('settings validation', () => {
  const schema = getSettingsFormSchema(t)

  test('current backend settings pass validation', () => {
    expect(schema.safeParse(settingsToFormValues(settings)).success).toBe(true)
  })

  test('values outside the backend ranges are rejected per field', () => {
    const values = {
      ...settingsToFormValues(settings),
      poll_interval_seconds: '10',
      sub2api_api_key_ids: '',
      tiers: [],
      sub2api_base_url: 'sub2api:8080',
    }
    const result = schema.safeParse(values)

    expect(result.success).toBe(false)
    const fields = result.error?.issues.map((issue) => issue.path[0])
    expect(fields).toEqual(
      expect.arrayContaining([
        'poll_interval_seconds',
        'sub2api_api_key_ids',
        'tiers',
        'sub2api_base_url',
      ])
    )
  })

  test('a ban group longer than 64 bytes is rejected', () => {
    const values = {
      ...settingsToFormValues(settings),
      ban_group: '封'.repeat(22),
    }

    expect(schema.safeParse(values).success).toBe(false)
  })
})

describe('ban terms validation', () => {
  const schema = getBanFormSchema(t)

  test('0 hours (until manual unban) and 8760 hours are accepted', () => {
    expect(schema.safeParse({ hours: '0', reason: '' }).success).toBe(true)
    expect(schema.safeParse({ hours: '8760', reason: '' }).success).toBe(true)
  })

  test('fractional, empty or too long durations are rejected', () => {
    expect(schema.safeParse({ hours: '1.5', reason: '' }).success).toBe(false)
    expect(schema.safeParse({ hours: '', reason: '' }).success).toBe(false)
    expect(schema.safeParse({ hours: '8761', reason: '' }).success).toBe(false)
  })
})

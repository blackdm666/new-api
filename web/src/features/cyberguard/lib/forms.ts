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
import { z } from 'zod'

import type { CyberGuardSettings, CyberGuardSettingsUpdate } from '../types'

// Numbers stay strings while editing so an empty input is a validation error
// instead of a silent 0; the payload builders convert them.

const MAX_BAN_HOURS = 8760

/** Parses "1, 9 37" into unique positive ids; null when any part is invalid. */
export function parseIdList(value: string): number[] | null {
  const ids: number[] = []
  for (const part of value.split(/[\s,，]+/)) {
    if (!part) continue
    if (!/^\d+$/.test(part)) return null
    const id = Number(part)
    if (id <= 0 || !Number.isSafeInteger(id)) return null
    if (!ids.includes(id)) ids.push(id)
  }
  return ids
}

function wholeNumber(min: number, max: number, message: string) {
  return z
    .string()
    .trim()
    .refine(
      (value) =>
        /^\d+$/.test(value) && Number(value) >= min && Number(value) <= max,
      message
    )
}

function idListText(min: number, max: number, message: string) {
  return z.string().refine((value) => {
    const ids = parseIdList(value)
    return ids !== null && ids.length >= min && ids.length <= max
  }, message)
}

export function getBanFormSchema(t: TFunction) {
  return z.object({
    hours: wholeNumber(
      0,
      MAX_BAN_HOURS,
      t('Enter whole hours from 0 to 8760.')
    ),
    reason: z
      .string()
      .trim()
      .max(500, t('Keep the reason within 500 characters.')),
  })
}

export type BanFormValues = z.infer<ReturnType<typeof getBanFormSchema>>

export function getAdoptFormSchema(t: TFunction) {
  return getBanFormSchema(t).extend({
    original_group: z
      .string()
      .trim()
      .min(1, t('Choose the group to restore on unban.')),
  })
}

export type AdoptFormValues = z.infer<ReturnType<typeof getAdoptFormSchema>>

export function getSettingsFormSchema(t: TFunction) {
  const outOfRange = t('Enter a whole number in the allowed range.')
  const tier = z.object({
    window_days: wholeNumber(1, 90, outOfRange),
    min_units: wholeNumber(1, 50, outOfRange),
    ban_hours: wholeNumber(0, MAX_BAN_HOURS, outOfRange),
  })
  return z.object({
    mode: z.enum(['off', 'observe', 'enforce']),
    sub2api_base_url: z
      .string()
      .trim()
      .max(255, t('Enter an http:// or https:// address.'))
      .regex(
        /^https?:\/\/[^\s?#]+$/,
        t('Enter an http:// or https:// address.')
      ),
    sub2api_admin_api_key: z
      .string()
      .trim()
      .max(512, t('The key is too long.'))
      .regex(/^\S*$/, t('The key cannot contain spaces.')),
    sub2api_api_key_ids: idListText(
      1,
      20,
      t('Enter 1 to 20 positive IDs separated by commas.')
    ),
    protected_channel_ids: idListText(
      1,
      200,
      t('Enter 1 to 200 positive IDs separated by commas.')
    ),
    poll_interval_seconds: wholeNumber(15, 3600, outOfRange),
    lookback_minutes: wholeNumber(1, 360, outOfRange),
    nosession_merge_seconds: wholeNumber(0, 3600, outOfRange),
    count_categories: z.array(z.enum(['cyber', 'bio'])),
    tiers: z
      .array(tier)
      .min(1, t('Keep 1 to 5 tiers.'))
      .max(5, t('Keep 1 to 5 tiers.')),
    ban_group: z
      .string()
      .trim()
      .refine(
        (value) => value !== '' && new TextEncoder().encode(value).length <= 64,
        t('Enter the ban group name (at most 64 bytes).')
      ),
    exempt_user_ids: idListText(
      0,
      200,
      t('Enter up to 200 positive IDs separated by commas.')
    ),
    email_enabled: z.boolean(),
    notify_root: z.boolean(),
  })
}

export type SettingsFormValues = z.infer<
  ReturnType<typeof getSettingsFormSchema>
>

export function settingsToFormValues(
  settings: CyberGuardSettings
): SettingsFormValues {
  return {
    mode: settings.mode,
    sub2api_base_url: settings.sub2api_base_url,
    sub2api_admin_api_key: '',
    sub2api_api_key_ids: settings.sub2api_api_key_ids.join(', '),
    protected_channel_ids: settings.protected_channel_ids.join(', '),
    poll_interval_seconds: String(settings.poll_interval_seconds),
    lookback_minutes: String(settings.lookback_minutes),
    nosession_merge_seconds: String(settings.nosession_merge_seconds),
    count_categories: settings.count_categories.filter(
      (category): category is 'cyber' | 'bio' =>
        category === 'cyber' || category === 'bio'
    ),
    tiers: settings.tiers.map((tier) => ({
      window_days: String(tier.window_days),
      min_units: String(tier.min_units),
      ban_hours: String(tier.ban_hours),
    })),
    ban_group: settings.ban_group,
    exempt_user_ids: settings.exempt_user_ids.join(', '),
    email_enabled: settings.email_enabled,
    notify_root: settings.notify_root,
  }
}

/** The admin key is sent only when a new one was typed (empty keeps it). */
export function settingsFormToPayload(
  values: SettingsFormValues
): CyberGuardSettingsUpdate {
  const payload: CyberGuardSettingsUpdate = {
    mode: values.mode,
    sub2api_base_url: values.sub2api_base_url.trim(),
    sub2api_api_key_ids: parseIdList(values.sub2api_api_key_ids) ?? [],
    protected_channel_ids: parseIdList(values.protected_channel_ids) ?? [],
    poll_interval_seconds: Number(values.poll_interval_seconds),
    lookback_minutes: Number(values.lookback_minutes),
    nosession_merge_seconds: Number(values.nosession_merge_seconds),
    count_categories: values.count_categories,
    tiers: values.tiers.map((tier) => ({
      window_days: Number(tier.window_days),
      min_units: Number(tier.min_units),
      ban_hours: Number(tier.ban_hours),
    })),
    ban_group: values.ban_group.trim(),
    exempt_user_ids: parseIdList(values.exempt_user_ids) ?? [],
    email_enabled: values.email_enabled,
    notify_root: values.notify_root,
  }
  const key = values.sub2api_admin_api_key.trim()
  if (key) payload.sub2api_admin_api_key = key
  return payload
}

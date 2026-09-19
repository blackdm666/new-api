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
import type { CustomOAuthBinding } from './oauth'

function scalarString(value: unknown): string | undefined {
  if (typeof value === 'string') return value
  if (typeof value === 'number' && Number.isFinite(value)) {
    return String(value)
  }
  return undefined
}

function numericId(value: unknown): number | undefined {
  if (typeof value === 'number' && Number.isFinite(value)) return value
  if (typeof value === 'string' && /^\d+$/.test(value)) return Number(value)
  return undefined
}

/**
 * Normalize the native backend response and older proxy-compatible shapes.
 * Invalid non-array payloads become an empty list so a malformed response
 * cannot take down the whole profile or admin users page.
 */
export function normalizeOAuthBindings(value: unknown): CustomOAuthBinding[] {
  if (!Array.isArray(value)) return []

  return value.flatMap((item) => {
    if (!item || typeof item !== 'object' || Array.isArray(item)) return []

    const raw = item as Record<string, unknown>
    const providerId = numericId(raw.provider_id)
    if (providerId === undefined) return []

    const providerUserId =
      scalarString(raw.provider_user_id) ?? scalarString(raw.external_id) ?? ''
    const providerName = scalarString(raw.provider_name) || String(providerId)
    const providerSlug = scalarString(raw.provider_slug) ?? ''
    const providerIcon = scalarString(raw.provider_icon) ?? ''

    return [
      {
        provider_id: providerId,
        provider_name: providerName,
        provider_slug: providerSlug,
        provider_icon: providerIcon,
        provider_user_id: providerUserId,
      },
    ]
  })
}

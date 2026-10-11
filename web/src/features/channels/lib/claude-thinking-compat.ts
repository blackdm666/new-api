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
import { CLAUDE_FIELD_PASSTHROUGH_TYPES } from '../constants'
import type { ClaudeThinkingCompatMode } from '../types'

export const CLAUDE_THINKING_COMPAT_MODES = [
  'auto',
  'always',
  'off',
] as const satisfies readonly ClaudeThinkingCompatMode[]

export const DEFAULT_CLAUDE_THINKING_COMPAT_MODE: ClaudeThinkingCompatMode =
  'auto'

const LEGACY_FIELD = 'claude_adaptive_thinking_compatibility'
const MODE_FIELD = 'claude_thinking_compat_mode'

/**
 * Channel types that forward native Claude Messages requests and therefore
 * honor the Claude thinking compatibility mode.
 */
export function supportsClaudeThinkingCompat(channelType: number): boolean {
  return CLAUDE_FIELD_PASSTHROUGH_TYPES.has(channelType)
}

export function isClaudeThinkingCompatMode(
  value: unknown
): value is ClaudeThinkingCompatMode {
  return (CLAUDE_THINKING_COMPAT_MODES as readonly unknown[]).includes(value)
}

/**
 * Reads the mode from stored channel settings. An explicit mode wins; the
 * deprecated channel-level or advanced custom route-level boolean opt-in
 * loads as `always`, matching the backend.
 */
export function readClaudeThinkingCompatMode(
  settings: Record<string, unknown>
): ClaudeThinkingCompatMode {
  const mode = settings[MODE_FIELD]
  if (isClaudeThinkingCompatMode(mode)) return mode
  if (settings[LEGACY_FIELD] === true) return 'always'

  const advancedCustom = settings.advanced_custom as
    | { advanced_routes?: unknown }
    | undefined
  const routes = Array.isArray(advancedCustom?.advanced_routes)
    ? (advancedCustom.advanced_routes as Record<string, unknown>[])
    : []
  if (routes.some((route) => route?.[LEGACY_FIELD] === true)) return 'always'

  return DEFAULT_CLAUDE_THINKING_COMPAT_MODE
}

/**
 * Writes the mode for channel types that support it and always drops the
 * deprecated channel-level boolean. Route-level booleans are dropped by the
 * advanced custom route normalization.
 */
export function writeClaudeThinkingCompatMode(
  settings: Record<string, unknown>,
  channelType: number,
  mode: ClaudeThinkingCompatMode | undefined
): void {
  delete settings[LEGACY_FIELD]
  if (!supportsClaudeThinkingCompat(channelType)) {
    delete settings[MODE_FIELD]
    return
  }
  settings[MODE_FIELD] = isClaudeThinkingCompatMode(mode)
    ? mode
    : DEFAULT_CLAUDE_THINKING_COMPAT_MODE
}

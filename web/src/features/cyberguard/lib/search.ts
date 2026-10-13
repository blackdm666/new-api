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
import { z } from 'zod'

export const CYBERGUARD_TABS = [
  'overview',
  'users',
  'events',
  'bans',
  'accounts',
  'settings',
] as const

export type CyberGuardTab = (typeof CYBERGUARD_TABS)[number]

/**
 * URL search params of /admin-cyberguard. `userId` narrows the events and
 * bans tabs to one user. Invalid values are dropped so stale links still open.
 */
export const cyberGuardSearchSchema = z.object({
  tab: z.enum(CYBERGUARD_TABS).optional().catch(undefined),
  userId: z.number().int().positive().optional().catch(undefined),
})

export type CyberGuardSearch = z.output<typeof cyberGuardSearchSchema>

/** Usage-log deep link around an event; logs are written within ±10 min. */
export function usageLogSearchForEvent(
  requestId: string,
  occurredAt: number
): { upstreamRequestId: string; startTime: number; endTime: number } {
  return {
    upstreamRequestId: requestId,
    startTime: (occurredAt - 3600) * 1000,
    endTime: (occurredAt + 3600) * 1000,
  }
}

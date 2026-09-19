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
import type { LogOtherData } from '../types'

export interface EffectiveGroupRatio {
  ratio: number
  isUserSpecific: boolean
}

export function getEffectiveGroupRatioInfo(
  other: LogOtherData | null | undefined
): EffectiveGroupRatio | null {
  const userGroupRatio = other?.user_group_ratio
  if (
    userGroupRatio != null &&
    userGroupRatio !== -1 &&
    Number.isFinite(userGroupRatio)
  ) {
    return { ratio: userGroupRatio, isUserSpecific: true }
  }

  const groupRatio = other?.group_ratio
  if (groupRatio != null && Number.isFinite(groupRatio)) {
    return { ratio: groupRatio, isUserSpecific: false }
  }

  return null
}

export function getEffectiveGroupRatio(
  other: LogOtherData | null | undefined
): number {
  return getEffectiveGroupRatioInfo(other)?.ratio ?? 1
}

export function applyLoggedGroupRatio(
  basePrice: number,
  other: LogOtherData | null | undefined
): number {
  return basePrice * getEffectiveGroupRatio(other)
}

export function formatRatioCompact(ratio: number | null | undefined): string {
  if (ratio == null || !Number.isFinite(ratio)) return '-'
  return Number.isInteger(ratio)
    ? String(ratio)
    : ratio.toFixed(4).replace(/\.?0+$/, '')
}

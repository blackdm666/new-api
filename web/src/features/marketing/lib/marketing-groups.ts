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
import type { Option } from '@/components/multi-select'

export function normalizeMarketingGroups(groups?: string[]): string[] {
  const normalized = new Set<string>()
  for (const group of groups ?? []) {
    const value = group.trim()
    if (value) normalized.add(value)
  }
  return [...normalized]
}

export function buildMarketingGroupOptions(
  configuredGroups: string[],
  selectedGroups: string[]
): Option[] {
  return normalizeMarketingGroups([...configuredGroups, ...selectedGroups])
    .sort((left, right) => left.localeCompare(right))
    .map((group) => ({ value: group, label: group }))
}

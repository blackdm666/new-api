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

/**
 * Moves the groups an administrator pinned to the bottom to the end of a group
 * record, in the pinned order. Other groups keep the order the server sent.
 * Group names that look like array indexes ("1", "2") are always enumerated
 * first by JavaScript objects and cannot be moved.
 */
export function orderGroupsPinnedBottom<T>(
  groups: Record<string, T>,
  pinBottom: readonly string[] | null | undefined
): Record<string, T> {
  const pinned = (pinBottom ?? []).filter((name) => Object.hasOwn(groups, name))
  if (pinned.length === 0) return groups

  const ordered: Record<string, T> = {}
  for (const [name, value] of Object.entries(groups)) {
    if (!pinned.includes(name)) ordered[name] = value
  }
  for (const name of pinned) {
    ordered[name] = groups[name]
  }
  return ordered
}

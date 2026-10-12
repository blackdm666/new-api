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
import type { TopNavLink } from '../types'

/**
 * Anchor attributes for an external top navigation link: a new tab by default,
 * the current tab for website pages that belong to the same site.
 */
export function getExternalLinkProps(link: Pick<TopNavLink, 'sameTab'>): {
  target?: '_blank'
  rel?: string
} {
  if (link.sameTab) return {}
  return { target: '_blank', rel: 'noopener noreferrer' }
}

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
declare global {
  interface Window {
    /** Google Tag Manager queue; only present when the site loads GTM. */
    dataLayer?: Array<Record<string, unknown>>
  }
}

/** Where a navigation click happened, as reported in `nav_click` events. */
export type NavClickLocation = 'console_header' | 'dropdown' | 'mobile_menu'

/**
 * Report a navigation click to Google Tag Manager. Does nothing when the page
 * did not load GTM, so callers never need to guard it.
 */
export function trackNavClick(
  navItem: string,
  navLocation: NavClickLocation
): void {
  if (typeof window === 'undefined' || !Array.isArray(window.dataLayer)) return
  window.dataLayer.push({
    event: 'nav_click',
    nav_item: navItem,
    nav_location: navLocation,
  })
}

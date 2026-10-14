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
import { normalizeInterfaceLanguage } from '@/i18n/languages'

export const INFINITE_CANVAS_NAME = 'Infinite Canvas'
export const INFINITE_CANVAS_URL = 'https://canvas.best/'

/**
 * Business cooperation pages of the static website, served outside the SPA.
 * Paths keep the trailing slash used by the website.
 */
export const BUSINESS_PATH = '/business/'
export const BUSINESS_ENTERPRISE_PATH = '/business/enterprise/'
/** Reseller partners (代理合作): downstream API sites using 88API as their upstream. */
export const BUSINESS_PARTNER_PATH = '/business/partner/'
/** Referral rewards (推广返佣): creators, group owners and influencers. */
export const BUSINESS_REFERRAL_PATH = '/business/referral/'

/** Referral commission range shown in the Referral Rewards menu entry. */
export const BUSINESS_REFERRAL_COMMISSION_RANGE = '5%–15%'

/**
 * Resolve a website page for the interface language: Simplified Chinese lives
 * at the root, Traditional Chinese under `/zh-tw/`, and every other language
 * uses the English pages under `/en/` until it gets its own translation.
 */
export function getLocalizedSitePath(
  language: string | null | undefined,
  path: string
): string {
  const interfaceLanguage = normalizeInterfaceLanguage(language)
  if (interfaceLanguage === 'zhCN') return path
  if (interfaceLanguage === 'zhTW') return `/zh-tw${path}`
  return `/en${path}`
}

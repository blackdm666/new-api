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
// 88api: the docs site serves each language under its own path prefix
// (https://88api.ai/zh/docs/, /en/docs/, /zh-tw/docs/ ...).
const DOCS_LANGUAGE_PREFIXES: Record<string, string> = {
  zhCN: 'zh',
  zhTW: 'zh-tw',
  en: 'en',
  fr: 'fr',
  ru: 'ru',
  ja: 'ja',
  vi: 'vi',
}

const KNOWN_DOCS_PREFIXES = new Set(Object.values(DOCS_LANGUAGE_PREFIXES))

/**
 * Point a configured docs link at the interface language. Only links whose
 * first path segment is already a docs language prefix are rewritten, so
 * plain links such as https://docs.newapi.pro stay untouched.
 */
export function localizeDocsLink(docsLink: string, language?: string): string {
  const prefix = language ? DOCS_LANGUAGE_PREFIXES[language] : undefined
  if (!prefix) return docsLink

  let url: URL
  try {
    url = new URL(docsLink)
  } catch {
    return docsLink
  }

  const segments = url.pathname.split('/')
  if (!KNOWN_DOCS_PREFIXES.has(segments[1]?.toLowerCase() ?? '')) {
    return docsLink
  }
  segments[1] = prefix
  url.pathname = segments.join('/')
  return url.toString()
}

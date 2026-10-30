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
import { describe, expect, test } from 'vitest'

import { localizeDocsLink } from '../docs-link'

const DOCS = 'https://88api.ai/zh/docs/'

describe('localizeDocsLink', () => {
  test.each([
    ['zhCN', 'https://88api.ai/zh/docs/'],
    ['zhTW', 'https://88api.ai/zh-tw/docs/'],
    ['en', 'https://88api.ai/en/docs/'],
    ['fr', 'https://88api.ai/fr/docs/'],
    ['ru', 'https://88api.ai/ru/docs/'],
    ['ja', 'https://88api.ai/ja/docs/'],
    ['vi', 'https://88api.ai/vi/docs/'],
  ])(
    'switches the language prefix for interface language %s',
    (lang, expected) => {
      expect(localizeDocsLink(DOCS, lang)).toBe(expected)
    }
  )

  test('replaces any known prefix, keeping the rest of the path and query', () => {
    expect(
      localizeDocsLink('https://88api.ai/zh-TW/docs/quick-start?x=1', 'ja')
    ).toBe('https://88api.ai/ja/docs/quick-start?x=1')
  })

  test('keeps links without a docs language prefix unchanged', () => {
    expect(localizeDocsLink('https://docs.newapi.pro', 'en')).toBe(
      'https://docs.newapi.pro'
    )
    expect(localizeDocsLink('https://example.com/docs/', 'ja')).toBe(
      'https://example.com/docs/'
    )
  })

  test('keeps the link when the language is unknown or the URL is invalid', () => {
    expect(localizeDocsLink(DOCS, 'de')).toBe(DOCS)
    expect(localizeDocsLink(DOCS, undefined)).toBe(DOCS)
    expect(localizeDocsLink('/docs', 'en')).toBe('/docs')
  })
})

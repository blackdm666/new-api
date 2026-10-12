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
import { readFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, test } from 'vitest'

const localesDirectory = resolve(
  dirname(fileURLToPath(import.meta.url)),
  '../locales'
)
const locales = ['en', 'zh', 'zh-TW', 'ja', 'fr', 'ru', 'vi'] as const

// Names of client apps the console links to. Users look for these exact
// labels in the apps themselves and in the docs, so they are never translated.
const productNames = [
  'CC Switch',
  'Cherry Studio',
  'Claude Code',
  'Codex',
  'Gemini CLI',
  'Grok Build',
]

function readTranslations(locale: string): Record<string, string> {
  const resource = JSON.parse(
    readFileSync(join(localesDirectory, `${locale}.json`), 'utf8')
  ) as { translation: Record<string, string> }
  return resource.translation
}

function countOccurrences(text: string, name: string): number {
  return text.split(name).length - 1
}

describe('locale terminology', () => {
  test.each(locales)(
    'keeps every product name from the English key verbatim in %s',
    (locale) => {
      const missing: string[] = []
      for (const [key, value] of Object.entries(readTranslations(locale))) {
        for (const name of productNames) {
          if (countOccurrences(value, name) < countOccurrences(key, name)) {
            missing.push(`${key} => ${value}`)
          }
        }
      }

      expect(missing).toEqual([])
    }
  )

  test('translates Application in zh-TW as an app rather than 套用 (apply)', () => {
    expect(readTranslations('zh-TW')['Application']).not.toContain('套用')
  })
})

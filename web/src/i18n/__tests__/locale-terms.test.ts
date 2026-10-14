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

// zh-TW renders "app"/"application" as 應用程式 or 應用. 套用 means "apply"
// there, so a Simplified→Traditional conversion that maps 应用 to 套用 is only
// right for keys about applying something.
const appTerm = /\bapp(?:s|lications?)?\b/i
const applyTerm = /\bappl(?:y|ies|ied|ying)\b/i

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

  test('never uses the non-word 套用程式 in zh-TW', () => {
    const misused = Object.entries(readTranslations('zh-TW'))
      .filter(([, value]) => value.includes('套用程式'))
      .map(([key, value]) => `${key} => ${value}`)

    expect(misused).toEqual([])
  })

  test('translates app and application keys in zh-TW without 套用 (apply)', () => {
    const misused = Object.entries(readTranslations('zh-TW'))
      .filter(
        ([key, value]) =>
          appTerm.test(key) && !applyTerm.test(key) && value.includes('套用')
      )
      .map(([key, value]) => `${key} => ${value}`)

    expect(misused).toEqual([])
  })
})

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
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, test } from 'vitest'

const currentDirectory = dirname(fileURLToPath(import.meta.url))
const webSource = resolve(currentDirectory, '../../..')
const sourceTargets = [
  resolve(currentDirectory, '..'),
  join(webSource, 'features/system-settings/integrations/invoice-section.tsx'),
]
const locales = ['zh', 'zh-TW', 'fr', 'ja', 'ru', 'vi'] as const

function listSourceFiles(target: string): string[] {
  if (statSync(target).isFile()) return [target]
  return readdirSync(target).flatMap((entry) =>
    listSourceFiles(join(target, entry))
  )
}

function collectInvoiceKeys(): string[] {
  const keys = new Set<string>()
  for (const file of sourceTargets
    .flatMap(listSourceFiles)
    .filter((name) => /\.(ts|tsx)$/.test(name))) {
    const source = readFileSync(file, 'utf8')
    for (const match of source.matchAll(/\bt\(\s*(['"])([\s\S]*?)\1/g)) {
      const key = match[2]
      if (!key.includes('${') && !key.includes('\n')) keys.add(key)
    }
    for (const match of source.matchAll(/labelKey:\s*(['"])(.*?)\1/g)) {
      keys.add(match[2])
    }
    for (const match of source.matchAll(
      /\[\s*['"]InvoiceFile[^'"]+['"]\s*,\s*['"]([^'"]+)['"]/g
    )) {
      keys.add(match[1])
    }
  }
  return [...keys]
}

describe('invoice translations', () => {
  test('every supported non-English locale has translated invoice keys', () => {
    const keys = collectInvoiceKeys()
    expect(keys.length).toBeGreaterThan(0)

    for (const locale of locales) {
      const resource = JSON.parse(
        readFileSync(join(webSource, `i18n/locales/${locale}.json`), 'utf8')
      ) as { translation: Record<string, string> }
      const missing = keys.filter(
        (key) =>
          !Object.hasOwn(resource.translation, key) ||
          resource.translation[key] === key
      )
      expect(missing, `${locale}: ${missing.join(', ')}`).toEqual([])
    }
  })
})

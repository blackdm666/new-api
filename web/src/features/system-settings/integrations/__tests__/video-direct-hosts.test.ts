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

import {
  formatVideoDirectHosts,
  isValidVideoDirectHost,
  parseVideoDirectHosts,
  serializeVideoDirectHosts,
} from '../video-direct-hosts'

const currentDirectory = dirname(fileURLToPath(import.meta.url))
const webSource = resolve(currentDirectory, '../../../..')

describe('video direct hosts', () => {
  test('round-trips the environment list into one host per line', () => {
    const env = '*.volces.com, store.vod-qcloud.com;*.Klingai.COM.\n'
    expect(parseVideoDirectHosts(env)).toEqual([
      '*.volces.com',
      'store.vod-qcloud.com',
      '*.klingai.com',
    ])
    expect(formatVideoDirectHosts(env)).toBe(
      '*.volces.com\nstore.vod-qcloud.com\n*.klingai.com'
    )
    expect(serializeVideoDirectHosts(formatVideoDirectHosts(env))).toBe(
      '*.volces.com,store.vod-qcloud.com,*.klingai.com'
    )
  })

  test('accepts only exact hosts or a leading wildcard', () => {
    for (const host of [
      'cdn.example.com',
      '*.example.com',
      'dashscope-a717.oss-accelerate.aliyuncs.com',
    ]) {
      expect(isValidVideoDirectHost(host), host).toBe(true)
    }
    for (const host of [
      '*',
      '*.com',
      'localhost',
      'https://cdn.example.com',
      'cdn.example.com/path',
      'cdn.example.com:443',
      '1.2.3.4',
      '*.*.example.com',
      'cdn*.example.com',
      '-cdn.example.com',
      'cdn..example.com',
      'cdn.example.123',
    ]) {
      expect(isValidVideoDirectHost(host), host).toBe(false)
    }
  })

  test('every supported locale translates the section', () => {
    const source = readFileSync(
      join(
        webSource,
        'features/system-settings/integrations/video-direct-hosts-section.tsx'
      ),
      'utf8'
    )
    const keys = [
      ...[...source.matchAll(/\bt\(\s*(['"])([\s\S]*?)\1/g)].map((m) => m[2]),
      'Save video direct hosts',
    ]
    expect(keys.length).toBeGreaterThan(5)
    for (const locale of ['zh', 'zh-TW', 'fr', 'ja', 'ru', 'vi']) {
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

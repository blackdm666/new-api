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
import assert from 'node:assert/strict'

import { describe, test } from 'vitest'

import {
  HEADER_NAV_DEFAULT,
  parseHeaderNavModules,
  serializeHeaderNavModules,
} from '../config'

describe('header navigation Infinite Canvas config', () => {
  test('keeps the existing button name and URL for old saved configs', () => {
    const config = parseHeaderNavModules('{"home":false}')

    assert.deepEqual(config.infiniteCanvas, HEADER_NAV_DEFAULT.infiniteCanvas)
    assert.equal(config.home, false)
  })

  test('round-trips a customized button name and URL', () => {
    const config = parseHeaderNavModules(
      JSON.stringify({
        infiniteCanvas: {
          enabled: true,
          name: '  创作工作台  ',
          url: '  https://canvas.example.com/workspace  ',
        },
      })
    )

    assert.deepEqual(config.infiniteCanvas, {
      enabled: true,
      name: '创作工作台',
      url: 'https://canvas.example.com/workspace',
    })
    assert.deepEqual(
      JSON.parse(serializeHeaderNavModules(config)).infiniteCanvas,
      config.infiniteCanvas
    )
  })

  test('falls back field-by-field when a saved link is incomplete', () => {
    const config = parseHeaderNavModules(
      JSON.stringify({ infiniteCanvas: { name: '', url: 123 } })
    )

    assert.deepEqual(config.infiniteCanvas, HEADER_NAV_DEFAULT.infiniteCanvas)
  })

  test('does not expose a non-HTTP saved destination to navigation', () => {
    const config = parseHeaderNavModules(
      JSON.stringify({
        infiniteCanvas: { name: '安全入口', url: 'javascript:alert(1)' },
      })
    )

    assert.deepEqual(config.infiniteCanvas, {
      enabled: true,
      name: '安全入口',
      url: HEADER_NAV_DEFAULT.infiniteCanvas.url,
    })
  })

  test('round-trips a disabled Infinite Canvas button', () => {
    const config = parseHeaderNavModules(
      JSON.stringify({
        infiniteCanvas: {
          enabled: false,
          name: '创作工作台',
          url: 'https://canvas.example.com',
        },
      })
    )

    assert.equal(config.infiniteCanvas.enabled, false)
    assert.equal(
      JSON.parse(serializeHeaderNavModules(config)).infiniteCanvas.enabled,
      false
    )
  })
})

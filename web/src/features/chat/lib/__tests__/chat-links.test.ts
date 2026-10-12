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
import { describe, expect, it } from 'vitest'

import { parseChatConfig, resolveChatUrl } from '../chat-links'

describe('parseChatConfig', () => {
  it('drops the legacy ccswitch marker and keeps the ids of the other presets', () => {
    const presets = parseChatConfig([
      {
        'Cherry Studio':
          'cherrystudio://providers/api-keys?data={cherryConfig}',
      },
      { 'CC Switch': 'ccswitch' },
      { 'Lobe Chat': 'https://chat.example.com/?key={key}' },
    ])

    expect(presets).toEqual([
      {
        id: '0',
        name: 'Cherry Studio',
        url: 'cherrystudio://providers/api-keys?data={cherryConfig}',
        type: 'custom-protocol',
      },
      {
        id: '2',
        name: 'Lobe Chat',
        url: 'https://chat.example.com/?key={key}',
        type: 'web',
      },
    ])
  })

  it('keeps an administrator-defined ccswitch:// import link as a custom protocol preset', () => {
    const presets = parseChatConfig([
      {
        'CC Switch Link': 'ccswitch://v1/import?resource=provider&apiKey={key}',
      },
    ])

    expect(presets).toEqual([
      {
        id: '0',
        name: 'CC Switch Link',
        url: 'ccswitch://v1/import?resource=provider&apiKey={key}',
        type: 'custom-protocol',
      },
    ])
  })
})

describe('resolveChatUrl', () => {
  it('imports Cherry Studio keys as a separate 88API provider on the API host', () => {
    const url = resolveChatUrl({
      template: 'cherrystudio://providers/api-keys?v=1&data={cherryConfig}',
      apiKey: 'test-only',
      serverAddress: 'https://88api.ai',
    })

    const data = new URL(url).searchParams.get('data') ?? ''
    expect(JSON.parse(atob(data))).toEqual({
      id: '88api',
      name: '88API',
      type: 'openai',
      baseUrl: 'https://api.88api.ai',
      apiKey: 'sk-test-only',
    })
  })
})

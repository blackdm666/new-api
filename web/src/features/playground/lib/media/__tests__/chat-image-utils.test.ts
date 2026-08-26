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

import type { ChatCompletionResponse } from '../../../types'
import { extractChatImageSources } from '../chat-image-utils'

function responseWithContent(content: string): ChatCompletionResponse {
  return {
    id: 'chat-image-test',
    object: 'chat.completion',
    created: 1,
    model: 'gemini-3.1-flash-image',
    choices: [
      {
        index: 0,
        message: { role: 'assistant', content },
        finish_reason: 'stop',
      },
    ],
  }
}

describe('extractChatImageSources', () => {
  test('extracts a base64 image even when markdown is split across lines', () => {
    const response = responseWithContent(
      '![image]\n(data:image/jpeg;base64,/9j/4AAQ\nSkZJRg==)'
    )

    expect(extractChatImageSources(response)).toEqual([
      'data:image/jpeg;base64,/9j/4AAQSkZJRg==',
    ])
  })

  test('extracts remote markdown image URLs and removes duplicates', () => {
    const response = responseWithContent(
      '![one](https://cdn.example.com/a.png)\n![two](https://cdn.example.com/a.png)'
    )

    expect(extractChatImageSources(response)).toEqual([
      'https://cdn.example.com/a.png',
    ])
  })
})

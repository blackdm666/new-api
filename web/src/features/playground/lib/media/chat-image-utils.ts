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
import type { ChatCompletionResponse } from '../../types'

const DATA_IMAGE_PATTERN = /data:image\/[a-z0-9.+-]+;base64,[a-z0-9+/=\s]+/gi
const MARKDOWN_IMAGE_URL_PATTERN = /!\[[^\]]*\]\(\s*(https?:\/\/[^)\s]+)\s*\)/gi

function extractFromText(content: string) {
  const sources: string[] = []

  for (const match of content.matchAll(DATA_IMAGE_PATTERN)) {
    const [header, payload = ''] = match[0].split(',', 2)
    sources.push(`${header},${payload.replaceAll(/\s+/g, '')}`)
  }
  for (const match of content.matchAll(MARKDOWN_IMAGE_URL_PATTERN)) {
    if (match[1]) sources.push(match[1])
  }

  return sources
}

export function extractChatImageSources(response: ChatCompletionResponse) {
  const sources = response.choices.flatMap((choice) =>
    extractFromText(choice.message.content)
  )
  return [...new Set(sources)]
}

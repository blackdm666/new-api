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
import { api } from '@/lib/api'

import { API_ENDPOINTS } from './constants'
import type {
  ChatCompletionRequest,
  ChatCompletionResponse,
  GroupOption,
  ImageGenerationResponse,
  ModelOption,
  PlaygroundModelMode,
  PlaygroundModelTransport,
  PlaygroundVideo,
} from './types'

/**
 * Send chat completion request (non-streaming)
 */
export async function sendChatCompletion(
  payload: ChatCompletionRequest,
  signal?: AbortSignal
): Promise<ChatCompletionResponse> {
  const res = await api.post(API_ENDPOINTS.CHAT_COMPLETIONS, payload, {
    signal,
    skipErrorHandler: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Get user available models
 */
export async function getUserModels(group: string): Promise<ModelOption[]> {
  const res = await api.get(API_ENDPOINTS.USER_MODELS, {
    params: { group, details: true },
  })
  const { data } = res

  if (!data.success || !Array.isArray(data.data)) {
    return []
  }

  return data.data.flatMap((item: unknown): ModelOption[] => {
    if (typeof item === 'string') {
      return [{ label: item, value: item, mode: 'chat', transport: 'chat' }]
    }
    if (!item || typeof item !== 'object') return []

    const candidate = item as {
      model?: unknown
      mode?: unknown
      transport?: unknown
    }
    if (typeof candidate.model !== 'string') return []
    if (!isPlaygroundModelMode(candidate.mode)) return []

    return [
      {
        label: candidate.model,
        value: candidate.model,
        mode: candidate.mode,
        transport: isPlaygroundModelTransport(candidate.transport)
          ? candidate.transport
          : candidate.mode,
      },
    ]
  })
}

function isPlaygroundModelTransport(
  value: unknown
): value is PlaygroundModelTransport {
  return value === 'chat' || value === 'image' || value === 'video'
}

function isPlaygroundModelMode(value: unknown): value is PlaygroundModelMode {
  return value === 'chat' || value === 'image' || value === 'video'
}

export async function generatePlaygroundImage(
  model: string,
  group: string,
  prompt: string,
  signal?: AbortSignal
): Promise<ImageGenerationResponse> {
  const res = await api.post(
    API_ENDPOINTS.IMAGE_GENERATIONS,
    { model, group, prompt },
    { signal, skipErrorHandler: true, skipBusinessError: true } as Record<
      string,
      unknown
    >
  )
  return res.data
}

export async function submitPlaygroundVideo(
  model: string,
  group: string,
  prompt: string,
  signal?: AbortSignal
): Promise<PlaygroundVideo> {
  const res = await api.post(API_ENDPOINTS.VIDEOS, { model, group, prompt }, {
    signal,
    skipErrorHandler: true,
    skipBusinessError: true,
  } as Record<string, unknown>)
  return res.data
}

export async function getPlaygroundVideo(
  taskId: string,
  signal?: AbortSignal
): Promise<PlaygroundVideo> {
  const res = await api.get(`${API_ENDPOINTS.VIDEOS}/${taskId}`, {
    signal,
    skipErrorHandler: true,
    skipBusinessError: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Get user groups
 */
export async function getUserGroups(): Promise<GroupOption[]> {
  const res = await api.get(API_ENDPOINTS.USER_GROUPS)
  const { data } = res

  if (!data.success || !data.data) {
    return []
  }

  const groupData = data.data as Record<string, { desc: string; ratio: number }>

  // label is for button display (name only); desc is for dropdown content
  return Object.entries(groupData).map(([group, info]) => ({
    label: group,
    value: group,
    ratio: info.ratio,
    desc: info.desc,
  }))
}

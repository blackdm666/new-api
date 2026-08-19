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
import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  generatePlaygroundImage,
  getPlaygroundVideo,
  sendChatCompletion,
  submitPlaygroundVideo,
} from '../api'
import { ERROR_MESSAGES } from '../constants'
import {
  getPlaygroundRequestErrorMessage,
  extractChatImageSources,
  parseRequestErrorDetails,
} from '../lib'
import type {
  MediaTestState,
  PlaygroundConfig,
  PlaygroundModelMode,
  PlaygroundModelTransport,
  PlaygroundVideo,
} from '../types'

const VIDEO_POLL_INTERVAL_MS = 2_500

function initialMediaState(): MediaTestState {
  return {
    status: 'idle',
    mode: 'image',
    prompt: '',
    images: [],
  }
}

function waitForNextPoll(signal: AbortSignal) {
  return new Promise<void>((resolve, reject) => {
    if (signal.aborted) {
      reject(new DOMException('Aborted', 'AbortError'))
      return
    }
    const handleAbort = () => {
      window.clearTimeout(timer)
      reject(new DOMException('Aborted', 'AbortError'))
    }
    const timer = window.setTimeout(() => {
      signal.removeEventListener('abort', handleAbort)
      resolve()
    }, VIDEO_POLL_INTERVAL_MS)
    signal.addEventListener('abort', handleAbort, { once: true })
  })
}

function imageSource(item: { url?: string; b64_json?: string }) {
  if (item.url) return item.url
  if (item.b64_json) return `data:image/png;base64,${item.b64_json}`
  return ''
}

function videoResultUrl(video: PlaygroundVideo, taskId: string) {
  const metadata = video.metadata ?? {}
  for (const key of ['url', 'video_url', 'result_url', 'output_url']) {
    if (typeof metadata[key] === 'string' && metadata[key]) {
      return metadata[key] as string
    }
  }
  return `/v1/videos/${encodeURIComponent(taskId)}/content`
}

export function useMediaTest(model: string, group: string) {
  const { t } = useTranslation()
  const [result, setResult] = useState<MediaTestState>(initialMediaState)
  const abortControllerRef = useRef<AbortController | null>(null)
  const generationRef = useRef(0)

  const stop = useCallback(() => {
    generationRef.current += 1
    abortControllerRef.current?.abort()
    abortControllerRef.current = null
    setResult(initialMediaState())
  }, [])

  useEffect(() => stop, [model, group, stop])

  const run = useCallback(
    async (
      prompt: string,
      mode: Exclude<PlaygroundModelMode, 'chat'>,
      transport: PlaygroundModelTransport,
      config: PlaygroundConfig
    ) => {
      const generation = generationRef.current + 1
      generationRef.current = generation
      abortControllerRef.current?.abort()
      const abortController = new AbortController()
      abortControllerRef.current = abortController
      setResult({
        status: 'generating',
        mode,
        prompt,
        images: [],
        progress: mode === 'video' ? 0 : undefined,
      })

      try {
        if (mode === 'image') {
          const images =
            transport === 'chat'
              ? extractChatImageSources(
                  await sendChatCompletion(
                    {
                      model: config.model,
                      group: config.group,
                      messages: [{ role: 'user', content: prompt }],
                      stream: false,
                    },
                    abortController.signal
                  )
                )
              : (
                  await generatePlaygroundImage(
                    config.model,
                    config.group,
                    prompt,
                    abortController.signal
                  )
                ).data.map(imageSource).filter(Boolean)
          if (generationRef.current !== generation) return
          if (images.length === 0) {
            throw new Error(t('Image not available'))
          }
          setResult({ status: 'success', mode, prompt, images })
          return
        }

        let video = await submitPlaygroundVideo(
          config.model,
          config.group,
          prompt,
          abortController.signal
        )
        const taskId = video.id || video.task_id
        if (!taskId) throw new Error(ERROR_MESSAGES.API_REQUEST_ERROR)

        while (video.status === 'queued' || video.status === 'in_progress') {
          if (generationRef.current !== generation) return
          setResult((current) => ({
            ...current,
            progress: video.progress ?? current.progress,
          }))
          await waitForNextPoll(abortController.signal)
          video = await getPlaygroundVideo(taskId, abortController.signal)
        }
        if (generationRef.current !== generation) return

        if (video.status === 'failed') {
          setResult({
            status: 'error',
            mode,
            prompt,
            images: [],
            errorCode: video.error?.code,
            errorMessage: video.error?.message || t('Failed'),
          })
          return
        }

        setResult({
          status: 'success',
          mode,
          prompt,
          images: [],
          progress: video.progress,
          videoUrl: videoResultUrl(video, taskId),
        })
      } catch (error: unknown) {
        if (
          abortController.signal.aborted ||
          generationRef.current !== generation
        ) {
          return
        }
        const details = parseRequestErrorDetails(error)
        setResult({
          status: 'error',
          mode,
          prompt,
          images: [],
          errorCode: details.errorCode,
          errorMessage: getPlaygroundRequestErrorMessage(details, t),
        })
      } finally {
        if (generationRef.current === generation) {
          abortControllerRef.current = null
        }
      }
    },
    [t]
  )

  return {
    result,
    run,
    stop,
    isGenerating: result.status === 'generating',
  }
}

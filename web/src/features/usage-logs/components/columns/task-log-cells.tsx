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
import { Music } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'

import { TASK_ACTIONS, TASK_STATUS } from '../../constants'
import type { LogOtherData, TaskLog } from '../../types'
import {
  AudioPreviewDialog,
  type AudioClip,
} from '../dialogs/audio-preview-dialog'
import { FailReasonDialog } from '../dialogs/fail-reason-dialog'
import { LogCostDisplay } from '../log-cost-display'
import { ModelBadge } from '../model-badge'
import { useUsageLogsContext } from '../usage-logs-provider'

function parseTaskData(data: unknown): unknown[] {
  if (Array.isArray(data)) return data
  if (typeof data !== 'string') return []
  try {
    const parsed = JSON.parse(data)
    return Array.isArray(parsed) ? parsed : []
  } catch {
    return []
  }
}

function AudioPreviewCell(props: { log: TaskLog }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const clips = useMemo(() => {
    const data = parseTaskData(props.log.data)
    return data.filter(
      (clip) =>
        clip &&
        typeof clip === 'object' &&
        (clip as Record<string, unknown>).audio_url
    )
  }, [props.log.data])

  if (clips.length === 0) return null

  return (
    <>
      <button
        type='button'
        className='group flex items-center gap-1 text-left text-xs'
        onClick={() => setOpen(true)}
      >
        <Music className='text-muted-foreground size-3' />
        <span className='text-foreground leading-snug group-hover:underline'>
          {t('Click to preview audio')}
        </span>
      </button>
      <AudioPreviewDialog
        open={open}
        onOpenChange={setOpen}
        clips={clips as AudioClip[]}
      />
    </>
  )
}

export function TaskChannelCell(props: { log: TaskLog }) {
  const { sensitiveVisible } = useUsageLogsContext()
  const channelId = props.log.channel_id
  if (!channelId) {
    return <span className='text-muted-foreground/60 text-xs'>-</span>
  }

  const channelIdDisplay = `#${channelId}`
  const channelName = sensitiveVisible ? props.log.channel_name : '••••'
  const channelDisplay = props.log.channel_name
    ? `${props.log.channel_name} ${channelIdDisplay}`
    : channelIdDisplay

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger
          render={<div className='flex max-w-[160px] flex-col gap-0.5' />}
        >
          <StatusBadge
            label={channelIdDisplay}
            autoColor={String(channelId)}
            copyText={String(channelId)}
            size='sm'
            showDot={false}
            className='w-fit font-mono'
          />
          {props.log.channel_name ? (
            <span className='text-muted-foreground/70 truncate [font-family:var(--font-body)] !text-xs'>
              {channelName}
            </span>
          ) : null}
        </TooltipTrigger>
        <TooltipContent>
          {sensitiveVisible ? channelDisplay : channelIdDisplay}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}

export function TaskModelCell(props: { log: TaskLog }) {
  const originModel = props.log.properties?.origin_model_name?.trim()
  const upstreamModel = props.log.properties?.upstream_model_name?.trim()
  const modelName = originModel || upstreamModel
  if (!modelName) {
    return <span className='text-muted-foreground/60 text-xs'>-</span>
  }

  const actualModel = upstreamModel !== modelName ? upstreamModel : undefined
  return <ModelBadge modelName={modelName} actualModel={actualModel} />
}

export function TaskCostCell(props: { log: TaskLog }) {
  const other: LogOtherData | null = props.log.billing_source
    ? { billing_source: props.log.billing_source }
    : null
  return <LogCostDisplay quota={props.log.quota || 0} other={other} />
}

function isVideoTask(log: TaskLog): boolean {
  return (
    log.action === TASK_ACTIONS.GENERATE ||
    log.action === TASK_ACTIONS.TEXT_GENERATE ||
    log.action === TASK_ACTIONS.FIRST_TAIL_GENERATE ||
    log.action === TASK_ACTIONS.REFERENCE_GENERATE ||
    log.action === TASK_ACTIONS.REMIX_GENERATE
  )
}

function getVideoHref(log: TaskLog): string {
  const resultUrl = log.result_url?.trim() || ''
  if (
    resultUrl.startsWith('https://') ||
    resultUrl.startsWith('http://') ||
    resultUrl.startsWith('/')
  ) {
    return resultUrl
  }
  return `/v1/videos/${encodeURIComponent(log.task_id)}/content`
}

export function TaskDetailsCell(props: { log: TaskLog }) {
  const { t } = useTranslation()
  const [dialogOpen, setDialogOpen] = useState(false)
  const isSuccess = props.log.status === TASK_STATUS.SUCCESS

  if (props.log.platform === 'suno' && isSuccess) {
    const data = parseTaskData(props.log.data)
    const hasAudio = data.some(
      (clip) =>
        clip &&
        typeof clip === 'object' &&
        (clip as Record<string, unknown>).audio_url
    )
    if (hasAudio) return <AudioPreviewCell log={props.log} />
  }

  if (isSuccess && isVideoTask(props.log) && props.log.task_id) {
    return (
      <a
        href={getVideoHref(props.log)}
        target='_blank'
        rel='noopener noreferrer'
        className='text-foreground text-xs hover:underline'
      >
        {t('Click to preview video')}
      </a>
    )
  }

  if (!props.log.fail_reason) {
    return <span className='text-muted-foreground/60 text-xs'>-</span>
  }

  return (
    <>
      <button
        type='button'
        className='group flex max-w-[200px] items-center gap-1 text-left text-xs'
        onClick={() => setDialogOpen(true)}
        title={t('Click to view full error message')}
      >
        <span className='truncate leading-snug text-red-600 group-hover:underline dark:text-red-400'>
          {props.log.fail_reason}
        </span>
      </button>
      <FailReasonDialog
        failReason={props.log.fail_reason}
        open={dialogOpen}
        onOpenChange={setDialogOpen}
      />
    </>
  )
}

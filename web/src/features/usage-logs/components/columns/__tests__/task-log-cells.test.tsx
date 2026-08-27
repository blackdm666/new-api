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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { formatLogQuota } from '@/lib/format'

import { TASK_ACTIONS, TASK_STATUS } from '../../../constants'
import type { TaskLog } from '../../../types'
import { UsageLogsProvider } from '../../usage-logs-provider'
import {
  TaskChannelCell,
  TaskCostCell,
  TaskDetailsCell,
  TaskModelCell,
} from '../task-log-cells'

vi.mock('@/lib/lobe-icon', () => ({
  getLobeIcon: (iconName: string) => <span data-icon={iconName} />,
}))

const getTaskPreviewURLMock = vi.hoisted(() => vi.fn())
vi.mock('../../../api', () => ({
  getTaskPreviewURL: getTaskPreviewURLMock,
}))

afterEach(() => {
  getTaskPreviewURLMock.mockReset()
  vi.restoreAllMocks()
})

const baseTask: TaskLog = {
  id: 1,
  user_id: 7,
  platform: '41',
  task_id: 'task_preview_123',
  action: TASK_ACTIONS.GENERATE,
  channel_id: 71,
  channel_name: 'Vertex video channel',
  quota: 500_000,
  submit_time: 1_787_808_000,
  status: TASK_STATUS.SUCCESS,
}

describe('task log cells', () => {
  test('shows the channel id and channel name together', () => {
    render(
      <UsageLogsProvider>
        <TaskChannelCell log={baseTask} />
      </UsageLogsProvider>
    )

    expect(screen.getByText('#71')).toBeVisible()
    expect(screen.getByText('Vertex video channel')).toBeVisible()
  })

  test('shows the requested model name', () => {
    render(
      <TaskModelCell
        log={{
          ...baseTask,
          properties: {
            origin_model_name: 'gemini-omni-flash',
            upstream_model_name: 'gemini-omni-flash-preview',
          },
        }}
      />
    )

    expect(screen.getByText('gemini-omni-flash')).toBeVisible()
  })

  test('formats the final task quota with the shared cost display', () => {
    const rendered = render(<TaskCostCell log={baseTask} />)

    const normalized = (rendered.container.textContent ?? '').replaceAll(
      /\s/g,
      ''
    )
    expect(normalized).toContain(
      formatLogQuota(baseTask.quota).replaceAll(/\s/g, '')
    )
  })

  test('opens a successful video with an on-demand direct storage url', async () => {
    const previewURL =
      'https://example.r2.cloudflarestorage.com/bucket/video.mp4?signed=true'
    const replace = vi.fn()
    const close = vi.fn()
    const previewWindow = {
      close,
      location: { replace },
      opener: window,
    }
    vi.spyOn(window, 'open').mockReturnValue(previewWindow as unknown as Window)
    getTaskPreviewURLMock.mockResolvedValue({
      success: true,
      data: { url: previewURL, expires_in: 604800 },
    })
    render(<TaskDetailsCell log={baseTask} />)

    const preview = screen.getByRole('button', {
      name: 'Click to preview video',
    })
    fireEvent.click(preview)

    expect(window.open).toHaveBeenCalledWith('about:blank', '_blank')
    await waitFor(() => {
      expect(getTaskPreviewURLMock).toHaveBeenCalledWith(baseTask.task_id)
      expect(replace).toHaveBeenCalledWith(previewURL)
    })
    expect(previewWindow.opener).toBeNull()
    expect(close).not.toHaveBeenCalled()
  })
})

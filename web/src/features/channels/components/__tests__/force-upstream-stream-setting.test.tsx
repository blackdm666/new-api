import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useForm } from 'react-hook-form'
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
import { assert, describe, expect, test } from 'vitest'

import { Form } from '@/components/ui/form'

import { CHANNEL_TYPE_GEMINI } from '../../constants'
import {
  buildSettingJSON,
  CHANNEL_FORM_DEFAULT_VALUES,
  transformChannelToFormDefaults,
  transformFormDataToCreatePayload,
  transformFormDataToUpdatePayload,
  type ChannelFormValues,
} from '../../lib/channel-form'
import { channelSchema } from '../../types'
import { ForceUpstreamStreamSetting } from '../force-upstream-stream-setting'

const switchName = 'Force upstream streaming'

function SettingsForm(props: {
  channelType: number
  disabled?: boolean
  enabled?: boolean
  onSave: (value: string) => void
}) {
  const form = useForm<ChannelFormValues>({
    defaultValues: {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      type: props.channelType,
      force_upstream_stream: props.enabled,
    },
  })
  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit((values) =>
          props.onSave(buildSettingJSON(values))
        )}
      >
        <ForceUpstreamStreamSetting
          channelType={props.channelType}
          disabled={props.disabled}
        />
        <button type='submit'>Save</button>
      </form>
    </Form>
  )
}

describe('Force upstream streaming channel setting', () => {
  test.each([undefined, false, true])(
    'Gemini channel preserves setting %s through create, update and reload',
    (enabled) => {
      const channel = channelSchema.parse({
        id: 1,
        name: 'Gemini channel',
        key: '',
        type: CHANNEL_TYPE_GEMINI,
        status: 1,
        created_time: 0,
        test_time: 0,
        response_time: 0,
        balance_updated_time: 0,
        setting: JSON.stringify({ force_upstream_stream: enabled }),
      })
      const values = transformChannelToFormDefaults(channel)
      const payloads = [
        transformFormDataToCreatePayload(values).channel,
        transformFormDataToUpdatePayload(values, channel.id),
      ]
      for (const payload of payloads) {
        assert(typeof payload.setting === 'string')
        expect(JSON.parse(payload.setting).force_upstream_stream).toBe(
          enabled === true
        )
        expect(
          transformChannelToFormDefaults({
            ...channel,
            setting: payload.setting,
          }).force_upstream_stream
        ).toBe(enabled === true)
      }
    }
  )

  test('new channels default to forced streaming disabled', () => {
    expect(
      JSON.parse(
        buildSettingJSON({
          ...CHANNEL_FORM_DEFAULT_VALUES,
          type: CHANNEL_TYPE_GEMINI,
        })
      ).force_upstream_stream
    ).toBe(false)
  })

  test('Gemini channel exposes an accessible switch and saves on and off', async () => {
    const user = userEvent.setup()
    const saved: string[] = []
    render(
      <SettingsForm
        channelType={CHANNEL_TYPE_GEMINI}
        onSave={(value) => saved.push(value)}
      />
    )
    const toggle = screen.getByRole('switch', { name: switchName })
    expect(toggle).not.toBeChecked()
    await user.click(toggle)
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(JSON.parse(saved[0]).force_upstream_stream).toBe(true)
    toggle.focus()
    await user.keyboard(' ')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(JSON.parse(saved[1]).force_upstream_stream).toBe(false)
  })

  test('non-Gemini channels hide the switch and do not save it as enabled', async () => {
    const user = userEvent.setup()
    const saved: string[] = []
    render(
      <SettingsForm
        channelType={1}
        enabled
        onSave={(value) => saved.push(value)}
      />
    )
    expect(screen.queryByRole('switch')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(JSON.parse(saved[0]).force_upstream_stream).toBe(false)
  })

  test('locked channel settings prevent toggling', async () => {
    const user = userEvent.setup()
    render(
      <SettingsForm
        channelType={CHANNEL_TYPE_GEMINI}
        disabled
        enabled
        onSave={() => undefined}
      />
    )
    const toggle = screen.getByRole('switch', { name: switchName })
    expect(toggle).toHaveAttribute('aria-disabled', 'true')
    await user.click(toggle)
    expect(toggle).toBeChecked()
  })
})

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
import { useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
} from '@/components/ui/form'
import { Switch } from '@/components/ui/switch'

import type { ChannelFormValues } from '../lib/channel-form'
import { supportsForceUpstreamStream } from '../lib/force-upstream-stream'

export function ForceUpstreamStreamSetting(props: {
  channelType: number
  disabled?: boolean
}) {
  const { t } = useTranslation()
  const form = useFormContext<ChannelFormValues>()
  if (!supportsForceUpstreamStream(props.channelType)) return null

  return (
    <FormField
      control={form.control}
      name='force_upstream_stream'
      render={({ field }) => (
        <FormItem className='flex items-center justify-between gap-4 px-4 py-3'>
          <div className='space-y-0.5'>
            <FormLabel>{t('Force upstream streaming')}</FormLabel>
            <FormDescription>
              {t(
                'Send non-streaming text requests upstream as streams and return one complete response. Use when the upstream cuts long non-streaming requests; image, speech and Responses requests are unaffected.'
              )}
            </FormDescription>
          </div>
          <FormControl>
            <Switch
              checked={field.value === true}
              onCheckedChange={field.onChange}
              disabled={props.disabled}
              onBlur={field.onBlur}
              name={field.name}
              ref={field.ref}
            />
          </FormControl>
        </FormItem>
      )}
    />
  )
}

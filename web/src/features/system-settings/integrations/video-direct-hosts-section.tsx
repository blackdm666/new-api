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
import { zodResolver } from '@hookform/resolvers/zod'
import { useMemo } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Textarea } from '@/components/ui/textarea'

import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'
import {
  MAX_VIDEO_DIRECT_HOSTS,
  MAX_VIDEO_DIRECT_HOSTS_LENGTH,
  VIDEO_DIRECT_HOSTS_OPTION_KEY,
  formatVideoDirectHosts,
  isValidVideoDirectHost,
  parseVideoDirectHosts,
  serializeVideoDirectHosts,
} from './video-direct-hosts'

const createSchema = (t: (key: string) => string) =>
  z.object({
    hosts: z
      .string()
      .refine(
        (value) =>
          serializeVideoDirectHosts(value).length <=
          MAX_VIDEO_DIRECT_HOSTS_LENGTH,
        t('The host list is too long')
      )
      .refine(
        (value) =>
          parseVideoDirectHosts(value).length <= MAX_VIDEO_DIRECT_HOSTS,
        t('Enter at most 256 hosts')
      )
      .refine(
        (value) => parseVideoDirectHosts(value).every(isValidVideoDirectHost),
        t(
          'Each line must be a host name such as cdn.example.com or *.example.com, without protocol, port, path or IP address'
        )
      ),
  })

type VideoDirectHostsFormValues = z.infer<ReturnType<typeof createSchema>>

type VideoDirectHostsSectionProps = {
  defaultValue: string
}

export function VideoDirectHostsSection({
  defaultValue,
}: VideoDirectHostsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const defaultValues = useMemo(
    () => ({ hosts: formatVideoDirectHosts(defaultValue) }),
    [defaultValue]
  )

  const form = useForm<VideoDirectHostsFormValues>({
    resolver: zodResolver(createSchema(t)),
    defaultValues,
  })

  useResetForm(form, defaultValues)

  const onSubmit = async (values: VideoDirectHostsFormValues) => {
    const next = serializeVideoDirectHosts(values.hosts)
    if (next === serializeVideoDirectHosts(defaultValue)) return
    await updateOption.mutateAsync({
      key: VIDEO_DIRECT_HOSTS_OPTION_KEY,
      value: next,
    })
  }

  return (
    <SettingsSection title={t('Video Direct Delivery')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)} autoComplete='off'>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save video direct hosts'
          />
          <FormField
            control={form.control}
            name='hosts'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Official video hosts')}</FormLabel>
                <FormControl>
                  <Textarea
                    rows={10}
                    spellCheck={false}
                    placeholder={'*.example.com\ncdn.example.net'}
                    className='font-mono'
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Successful task videos hosted on these HTTPS hosts are returned to clients directly; all other videos are archived first. One host per line; *.example.com matches subdomains but not example.com itself.'
                  )}
                </FormDescription>
                <FormDescription>
                  {t(
                    'Changes apply to all nodes within the option sync interval without a restart. Until saved here, the TASK_VIDEO_DIRECT_HOSTS environment variable is shown and used; saving an empty list archives every video.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}

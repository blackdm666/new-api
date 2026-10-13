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
import type { Control } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Checkbox } from '@/components/ui/checkbox'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
} from '@/components/ui/form'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'

import type { SettingsFormValues } from '../lib/forms'
import { categoryLabel } from '../lib/labels'
import { SettingsTextField } from './settings-text-field'

const COUNTABLE = ['cyber', 'bio'] as const

/** Which refusal categories count toward a ban. */
export function SettingsCategoriesField(props: {
  control: Control<SettingsFormValues>
}) {
  const { t } = useTranslation()
  return (
    <FormField
      control={props.control}
      name='count_categories'
      render={({ field }) => (
        <FormItem>
          <div className='text-sm font-medium'>
            {t('Categories that count')}
          </div>
          <div className='flex flex-wrap gap-4'>
            {COUNTABLE.map((category) => (
              <Label
                key={category}
                htmlFor={`cyberguard-count-${category}`}
                className='flex items-center gap-2 font-normal'
              >
                <Checkbox
                  id={`cyberguard-count-${category}`}
                  checked={field.value.includes(category)}
                  onCheckedChange={(checked) =>
                    field.onChange(
                      checked
                        ? [...field.value, category]
                        : field.value.filter((value) => value !== category)
                    )
                  }
                />
                {categoryLabel(category, t)}
              </Label>
            ))}
          </div>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Usage-policy refusals and blocked sessions are always recorded only.'
            )}
          </p>
        </FormItem>
      )}
    />
  )
}

function SwitchField(props: {
  control: Control<SettingsFormValues>
  name: 'email_enabled' | 'notify_root'
  label: string
  description: string
}) {
  return (
    <FormField
      control={props.control}
      name={props.name}
      render={({ field }) => (
        <FormItem className='flex items-start justify-between gap-4 rounded-lg border p-3'>
          <div className='space-y-1'>
            <FormLabel>{props.label}</FormLabel>
            <FormDescription>{props.description}</FormDescription>
          </div>
          <FormControl>
            <Switch checked={field.value} onCheckedChange={field.onChange} />
          </FormControl>
        </FormItem>
      )}
    />
  )
}

/** Where banned users go, who is never banned, and who is told about it. */
export function SettingsBanFields(props: {
  control: Control<SettingsFormValues>
}) {
  const { t } = useTranslation()
  return (
    <>
      <div className='grid gap-4 md:grid-cols-2'>
        <SettingsTextField
          control={props.control}
          name='ban_group'
          label={t('Ban group')}
          description={t(
            'Existing group whose usable groups exclude GPT. Banned users are moved here.'
          )}
        />
        <SettingsTextField
          control={props.control}
          name='exempt_user_ids'
          label={t('Exempt user IDs')}
          description={t('Never counted or banned. Comma separated.')}
        />
      </div>
      <SwitchField
        control={props.control}
        name='email_enabled'
        label={t('Send the ban email')}
        description={t(
          'Uses the editable "Account restriction notice" email template. Unbanning sends no email.'
        )}
      />
      <SwitchField
        control={props.control}
        name='notify_root'
        label={t('Notify the root administrator')}
        description={t(
          'Sends a notification for each automatic ban or new proposal.'
        )}
      />
    </>
  )
}

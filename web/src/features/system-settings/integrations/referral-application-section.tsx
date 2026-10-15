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
import { Input } from '@/components/ui/input'

import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'

const EMAIL_PATTERN = /^[^\s@<>"]+@[^\s@<>"]+\.[^\s@<>"]+$/

/** Splits the recipient option the same way the backend does. */
function parseReferralApplicationRecipients(value: string): string[] {
  return value
    .split(/[\s,;]+/)
    .map((item) => item.trim())
    .filter(Boolean)
}

const createReferralApplicationSchema = (t: (key: string) => string) =>
  z.object({
    ReferralApplicationRecipient: z.string().refine((value) => {
      const recipients = parseReferralApplicationRecipients(value)
      return (
        recipients.length > 0 &&
        recipients.every((item) => EMAIL_PATTERN.test(item))
      )
    }, t('Enter one or more valid email addresses separated by commas')),
  })

type ReferralApplicationFormValues = z.infer<
  ReturnType<typeof createReferralApplicationSchema>
>

type ReferralApplicationSectionProps = {
  defaultValues: ReferralApplicationFormValues
}

export function ReferralApplicationSection(
  props: ReferralApplicationSectionProps
) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const form = useForm<ReferralApplicationFormValues>({
    resolver: zodResolver(createReferralApplicationSchema(t)),
    defaultValues: props.defaultValues,
  })

  useResetForm(form, props.defaultValues)

  const onSubmit = async (values: ReferralApplicationFormValues) => {
    const recipients = parseReferralApplicationRecipients(
      values.ReferralApplicationRecipient
    ).join(', ')
    if (
      recipients ===
      parseReferralApplicationRecipients(
        props.defaultValues.ReferralApplicationRecipient
      ).join(', ')
    ) {
      return
    }
    await updateOption.mutateAsync({
      key: 'ReferralApplicationRecipient',
      value: recipients,
    })
  }

  return (
    <SettingsSection title={t('Referral Applications')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)} autoComplete='off'>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save referral application settings'
          />
          <FormField
            control={form.control}
            name='ReferralApplicationRecipient'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Application recipient email')}</FormLabel>
                <FormControl>
                  <Input
                    type='text'
                    inputMode='email'
                    placeholder='laohei@88fk.org'
                    autoComplete='off'
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Referral program applications submitted on the official website are emailed to these addresses through the notification SMTP profile. Separate multiple addresses with commas.'
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

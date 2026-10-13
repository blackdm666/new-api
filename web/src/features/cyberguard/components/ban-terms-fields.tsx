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
import type { UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'

import type { BanFormValues } from '../lib/forms'

/** Duration and reason inputs shared by the manual-ban and import forms. */
export function BanTermsFields(props: {
  // Narrowed to the shared fields so the import form can pass its own form.
  form: Pick<UseFormReturn<BanFormValues>, 'register' | 'formState'>
  idPrefix: string
  disabled: boolean
  reasonPlaceholder: string
}) {
  const { t } = useTranslation()
  const errors = props.form.formState.errors
  const hoursId = `${props.idPrefix}-hours`
  const reasonId = `${props.idPrefix}-reason`
  return (
    <>
      <Field data-invalid={Boolean(errors.hours)}>
        <FieldLabel htmlFor={hoursId}>{t('Ban duration (hours)')}</FieldLabel>
        <Input
          id={hoursId}
          type='number'
          inputMode='numeric'
          min={0}
          max={8760}
          disabled={props.disabled}
          aria-invalid={Boolean(errors.hours)}
          {...props.form.register('hours')}
        />
        <FieldDescription>
          {t('0 keeps the ban until you unban by hand. 72 hours = 3 days.')}
        </FieldDescription>
        <FieldError errors={[errors.hours]} />
      </Field>
      <Field data-invalid={Boolean(errors.reason)}>
        <FieldLabel htmlFor={reasonId}>{t('Reason')}</FieldLabel>
        <Textarea
          id={reasonId}
          rows={3}
          maxLength={500}
          placeholder={props.reasonPlaceholder}
          disabled={props.disabled}
          aria-invalid={Boolean(errors.reason)}
          {...props.form.register('reason')}
        />
        <FieldDescription>
          {t('Shown in the ban records. Leave empty to use the default.')}
        </FieldDescription>
        <FieldError errors={[errors.reason]} />
      </Field>
    </>
  )
}

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
import { X } from 'lucide-react'
import { useForm, type UseFormRegisterReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'

import { SettingsControlGroup } from '../components/settings-form-layout'
import {
  createMarketingEmailAccountSchema,
  type MarketingEmailAccountFormValues,
} from '../lib/marketing-email-account-schema'

type Props = {
  initialValues: MarketingEmailAccountFormValues
  editing: boolean
  saving: boolean
  onCancel: () => void
  onSave: (values: MarketingEmailAccountFormValues) => void
}

export function MarketingEmailAccountEditor(props: Props) {
  const { t } = useTranslation()
  const form = useForm<MarketingEmailAccountFormValues>({
    resolver: zodResolver(createMarketingEmailAccountSchema(t)),
    defaultValues: props.initialValues,
  })
  const save = form.handleSubmit((values) => {
    if (!props.editing && !values.token) {
      form.setError('token', {
        type: 'manual',
        message: t('Password or access token is required'),
      })
      return
    }
    props.onSave(values)
  })

  return (
    <SettingsControlGroup className='space-y-5'>
      <div className='flex items-center justify-between gap-3'>
        <p className='font-medium'>
          {props.editing
            ? t('Edit marketing account')
            : t('Add marketing account')}
        </p>
        <Button
          type='button'
          size='icon'
          variant='ghost'
          onClick={props.onCancel}
        >
          <X className='size-4' aria-hidden='true' />
          <span className='sr-only'>{t('Close')}</span>
        </Button>
      </div>
      <div className='grid gap-4 sm:grid-cols-2'>
        <AccountInput
          id='marketing-account-name'
          label={t('Account name')}
          error={form.formState.errors.name?.message}
          registration={form.register('name')}
        />
        <AccountInput
          id='marketing-account-host'
          label={t('SMTP Host')}
          error={form.formState.errors.server?.message}
          registration={form.register('server')}
        />
        <AccountInput
          id='marketing-account-port'
          label={t('Port')}
          type='number'
          error={form.formState.errors.port?.message}
          registration={form.register('port', { valueAsNumber: true })}
        />
        <AccountInput
          id='marketing-account-username'
          label={t('Username')}
          error={form.formState.errors.account?.message}
          registration={form.register('account')}
        />
        <AccountInput
          id='marketing-account-from'
          label={t('From Address')}
          type='email'
          error={form.formState.errors.from?.message}
          registration={form.register('from')}
        />
        <AccountInput
          id='marketing-account-token'
          label={t('Password / Access Token')}
          type='password'
          placeholder={
            props.editing
              ? t('Leave blank to keep the existing credential')
              : ''
          }
          error={form.formState.errors.token?.message}
          registration={form.register('token')}
        />
        <AccountInput
          id='marketing-account-weight'
          label={t('Weight')}
          type='number'
          error={form.formState.errors.weight?.message}
          registration={form.register('weight', { valueAsNumber: true })}
        />
        <AccountInput
          id='marketing-account-rpm'
          label={t('RPM limit')}
          type='number'
          error={form.formState.errors.rate_limit_per_minute?.message}
          registration={form.register('rate_limit_per_minute', {
            valueAsNumber: true,
          })}
        />
      </div>
      <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-4'>
        <AccountSwitch
          label='SSL/TLS'
          checked={form.watch('ssl_enabled')}
          onChange={(checked) => {
            form.setValue('ssl_enabled', checked, { shouldDirty: true })
            if (checked) {
              form.setValue('starttls_enabled', false, { shouldDirty: true })
            }
          }}
        />
        <AccountSwitch
          label='STARTTLS'
          checked={form.watch('starttls_enabled')}
          error={form.formState.errors.starttls_enabled?.message}
          onChange={(checked) => {
            form.setValue('starttls_enabled', checked, { shouldDirty: true })
            if (checked) {
              form.setValue('ssl_enabled', false, { shouldDirty: true })
            }
          }}
        />
        <AccountSwitch
          label={t('Skip TLS verification')}
          checked={form.watch('insecure_skip_verify')}
          onChange={(checked) =>
            form.setValue('insecure_skip_verify', checked, {
              shouldDirty: true,
            })
          }
        />
        <AccountSwitch
          label={t('Force AUTH LOGIN')}
          checked={form.watch('force_auth_login')}
          onChange={(checked) =>
            form.setValue('force_auth_login', checked, { shouldDirty: true })
          }
        />
      </div>
      <div className='flex justify-end gap-2'>
        <Button type='button' variant='outline' onClick={props.onCancel}>
          {t('Cancel')}
        </Button>
        <Button
          type='button'
          disabled={props.saving}
          onClick={() => void save()}
        >
          {t('Save')}
        </Button>
      </div>
    </SettingsControlGroup>
  )
}

function AccountInput(props: {
  id: string
  label: string
  type?: string
  placeholder?: string
  error?: string
  registration: UseFormRegisterReturn
}) {
  return (
    <div className='space-y-1.5 text-sm'>
      <label className='font-medium' htmlFor={props.id}>
        {props.label}
      </label>
      <Input
        id={props.id}
        type={props.type}
        placeholder={props.placeholder}
        aria-invalid={Boolean(props.error)}
        aria-describedby={props.error ? `${props.id}-error` : undefined}
        {...props.registration}
      />
      {props.error ? (
        <p id={`${props.id}-error`} className='text-destructive text-xs'>
          {props.error}
        </p>
      ) : null}
    </div>
  )
}

function AccountSwitch(props: {
  label: string
  checked: boolean
  error?: string
  onChange: (checked: boolean) => void
}) {
  return (
    <div className='space-y-1'>
      <div className='flex items-center justify-between gap-3 rounded-lg border px-3 py-2'>
        <span className='text-sm'>{props.label}</span>
        <Switch
          aria-label={props.label}
          checked={props.checked}
          onCheckedChange={props.onChange}
        />
      </div>
      {props.error ? (
        <p className='text-destructive text-xs'>{props.error}</p>
      ) : null}
    </div>
  )
}

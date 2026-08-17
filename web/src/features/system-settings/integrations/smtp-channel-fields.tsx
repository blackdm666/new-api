/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import type { FieldPath, UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'

import {
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import type { EmailFormValues } from './email-settings-section'

type SmtpSecurityMode = 'none' | 'ssl_tls' | 'starttls'

export type SmtpChannelFieldNames = {
  server: FieldPath<EmailFormValues>
  port: FieldPath<EmailFormValues>
  account: FieldPath<EmailFormValues>
  from: FieldPath<EmailFormValues>
  token: FieldPath<EmailFormValues>
  sslEnabled: FieldPath<EmailFormValues>
  startTLSEnabled: FieldPath<EmailFormValues>
  insecureSkipVerify: FieldPath<EmailFormValues>
  forceAuthLogin: FieldPath<EmailFormValues>
}

type Props = {
  form: UseFormReturn<EmailFormValues>
  names: SmtpChannelFieldNames
  disabled?: boolean
}

function securityMode(sslEnabled: boolean, startTLSEnabled: boolean) {
  if (sslEnabled) return 'ssl_tls'
  if (startTLSEnabled) return 'starttls'
  return 'none'
}

export function SmtpChannelFields({ form, names, disabled = false }: Props) {
  const { t } = useTranslation()
  const sslEnabled = Boolean(form.watch(names.sslEnabled))
  const startTLSEnabled = Boolean(form.watch(names.startTLSEnabled))

  return (
    <div className='grid min-w-0 gap-x-5 gap-y-6 lg:grid-cols-2'>
      <FormField
        control={form.control}
        name={names.server}
        render={({ field }) => (
          <FormItem className='lg:col-span-2'>
            <FormLabel>{t('SMTP Host')}</FormLabel>
            <FormControl>
              <Input
                autoComplete='off'
                placeholder={t('smtp.example.com')}
                disabled={disabled}
                value={String(field.value ?? '')}
                onBlur={field.onBlur}
                onChange={(event) => field.onChange(event.target.value)}
              />
            </FormControl>
            <FormDescription>
              {t('Hostname or IP of your SMTP provider')}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />

      <FormField
        control={form.control}
        name={names.port}
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('Port')}</FormLabel>
            <FormControl>
              <Input
                autoComplete='off'
                type='number'
                min={1}
                max={65535}
                placeholder='587'
                disabled={disabled}
                value={String(field.value ?? '')}
                onBlur={field.onBlur}
                onChange={(event) => field.onChange(event.target.value)}
              />
            </FormControl>
            <FormDescription>
              {t('Common ports include 25, 465, and 587')}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />

      <FormItem>
        <FormLabel>{t('SMTP encryption')}</FormLabel>
        <Select
          items={[
            { value: 'none', label: t('No encryption') },
            { value: 'ssl_tls', label: t('SSL/TLS') },
            { value: 'starttls', label: t('STARTTLS') },
          ]}
          value={securityMode(sslEnabled, startTLSEnabled)}
          disabled={disabled}
          onValueChange={(value) => {
            const mode = value as SmtpSecurityMode
            form.setValue(names.sslEnabled, mode === 'ssl_tls', {
              shouldDirty: true,
            })
            form.setValue(names.startTLSEnabled, mode === 'starttls', {
              shouldDirty: true,
            })
          }}
        >
          <FormControl>
            <SelectTrigger className='w-full'>
              <SelectValue />
            </SelectTrigger>
          </FormControl>
          <SelectContent alignItemWithTrigger={false}>
            <SelectGroup>
              <SelectItem value='none'>{t('No encryption')}</SelectItem>
              <SelectItem value='ssl_tls'>{t('SSL/TLS')}</SelectItem>
              <SelectItem value='starttls'>{t('STARTTLS')}</SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
        <FormDescription>
          {t('Choose one SMTP transport security mode')}
        </FormDescription>
      </FormItem>

      <FormField
        control={form.control}
        name={names.insecureSkipVerify}
        render={({ field }) => (
          <SettingsSwitchItem>
            <SettingsSwitchContent>
              <FormLabel>
                {t('Skip SMTP TLS certificate verification')}
              </FormLabel>
              <FormDescription>
                {t(
                  'Allow self-signed or hostname-mismatched SMTP certificates'
                )}
              </FormDescription>
            </SettingsSwitchContent>
            <FormControl>
              <Switch
                checked={Boolean(field.value)}
                onCheckedChange={field.onChange}
                disabled={disabled}
              />
            </FormControl>
          </SettingsSwitchItem>
        )}
      />

      <FormField
        control={form.control}
        name={names.forceAuthLogin}
        render={({ field }) => (
          <SettingsSwitchItem>
            <SettingsSwitchContent>
              <FormLabel>{t('Force AUTH LOGIN')}</FormLabel>
              <FormDescription>
                {t('Force SMTP authentication using AUTH LOGIN method')}
              </FormDescription>
            </SettingsSwitchContent>
            <FormControl>
              <Switch
                checked={Boolean(field.value)}
                onCheckedChange={field.onChange}
                disabled={disabled}
              />
            </FormControl>
          </SettingsSwitchItem>
        )}
      />

      <FormField
        control={form.control}
        name={names.account}
        render={({ field }) => (
          <FormItem className='lg:col-span-2'>
            <FormLabel>{t('Username')}</FormLabel>
            <FormControl>
              <Input
                autoComplete='off'
                placeholder={t('noreply@example.com')}
                disabled={disabled}
                value={String(field.value ?? '')}
                onBlur={field.onBlur}
                onChange={(event) => field.onChange(event.target.value)}
              />
            </FormControl>
            <FormDescription>
              {t('Account used when authenticating with the SMTP server')}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />

      <FormField
        control={form.control}
        name={names.from}
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('From Address')}</FormLabel>
            <FormControl>
              <Input
                autoComplete='off'
                type='email'
                placeholder={t('noreply@example.com')}
                disabled={disabled}
                value={String(field.value ?? '')}
                onBlur={field.onBlur}
                onChange={(event) => field.onChange(event.target.value)}
              />
            </FormControl>
            <FormDescription>
              {t('Envelope sender used for outgoing messages')}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />

      <FormField
        control={form.control}
        name={names.token}
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('Password / Access Token')}</FormLabel>
            <FormControl>
              <Input
                autoComplete='new-password'
                type='password'
                placeholder={t('Enter new token to update')}
                disabled={disabled}
                value={String(field.value ?? '')}
                onBlur={field.onBlur}
                onChange={(event) => field.onChange(event.target.value)}
              />
            </FormControl>
            <FormDescription>
              {t('Leave blank to keep the existing credential')}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
    </div>
  )
}

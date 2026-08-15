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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useEffect, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { getUserFacingErrorMessage } from '@/lib/user-facing-error'

import { testInvoiceStorage, updateInvoiceSettings } from '../api'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import type { InvoiceSettingsPayload } from '../types'

type StorageType = 'local' | 'oss' | 's3' | 'cos'

const STORAGE_FIELDS = {
  oss: [
    ['InvoiceFileOSSEndpoint', 'Endpoint', false],
    ['InvoiceFileOSSBucket', 'Bucket', false],
    ['InvoiceFileOSSRegion', 'Region', false],
    ['InvoiceFileOSSAccessKeyId', 'Access Key ID', false],
    ['InvoiceFileOSSAccessKeySecret', 'Access Key Secret', true],
    ['InvoiceFileOSSCustomDomain', 'Custom domain', false],
  ],
  s3: [
    ['InvoiceFileS3Endpoint', 'Endpoint', false],
    ['InvoiceFileS3Bucket', 'Bucket', false],
    ['InvoiceFileS3Region', 'Region', false],
    ['InvoiceFileS3AccessKeyId', 'Access Key ID', false],
    ['InvoiceFileS3AccessKeySecret', 'Access Key Secret', true],
  ],
  cos: [
    ['InvoiceFileCOSEndpoint', 'Endpoint', false],
    ['InvoiceFileCOSBucket', 'Bucket', false],
    ['InvoiceFileCOSRegion', 'Region', false],
    ['InvoiceFileCOSSecretId', 'Secret ID', false],
    ['InvoiceFileCOSSecretKey', 'Secret Key', true],
    ['InvoiceFileCOSCustomDomain', 'Custom domain', false],
  ],
} as const

type VisibleStorageOptionKey =
  (typeof STORAGE_FIELDS)[keyof typeof STORAGE_FIELDS][number][0]
type StorageOptionKey = VisibleStorageOptionKey | 'InvoiceFileS3CustomDomain'

const storageOptionKeys = [
  ...Object.values(STORAGE_FIELDS).flatMap((fields) =>
    fields.map(([key]) => key)
  ),
  'InvoiceFileS3CustomDomain',
] as StorageOptionKey[]

type Props = {
  defaultValues: {
    InvoiceApplicationNotifyAdminEnabled: boolean
    InvoiceIssuedNotifyUserEnabled: boolean
    InvoiceAdminEmail: string
    InvoiceMinimumAmountCents: string
    InvoiceDataRetentionDays: string
    InvoicePendingExpiryDays: string
    InvoiceFileEnabled: boolean
    InvoiceFileStorage: string
    InvoiceFileMaxSize: string
    InvoiceFileMaxCount: string
    InvoiceFileAllowedExts: string
    InvoiceFileLocalPath: string
    InvoiceFileSignedURLTTL: string
  } & Record<StorageOptionKey, string>
}

type FormValues = Props['defaultValues'] & {
  InvoiceMinimumAmountYuan: string
  InvoiceFileMaxSizeMiB: string
}

function decimalString(value: number, fallback: number): string {
  return Number.isFinite(value) ? String(value) : String(fallback)
}

function initialValues(defaultValues: Props['defaultValues']): FormValues {
  return {
    ...defaultValues,
    InvoiceMinimumAmountCents:
      defaultValues.InvoiceMinimumAmountCents || '50000',
    InvoiceFileMaxSize: defaultValues.InvoiceFileMaxSize || '5242880',
    InvoiceMinimumAmountYuan: decimalString(
      Number(defaultValues.InvoiceMinimumAmountCents || 50000) / 100,
      500
    ),
    InvoiceFileMaxSizeMiB: decimalString(
      Number(defaultValues.InvoiceFileMaxSize || 5242880) / 1024 / 1024,
      5
    ),
    ...Object.fromEntries(
      storageOptionKeys.map((key) => [key, defaultValues[key] ?? ''])
    ),
  }
}

function buildPayload(values: FormValues): InvoiceSettingsPayload {
  return {
    InvoiceApplicationNotifyAdminEnabled:
      values.InvoiceApplicationNotifyAdminEnabled,
    InvoiceIssuedNotifyUserEnabled: values.InvoiceIssuedNotifyUserEnabled,
    InvoiceAdminEmail: values.InvoiceAdminEmail,
    InvoiceMinimumAmountCents: Math.round(
      Number(values.InvoiceMinimumAmountYuan) * 100
    ),
    InvoiceDataRetentionDays: Number(values.InvoiceDataRetentionDays),
    InvoicePendingExpiryDays: Number(values.InvoicePendingExpiryDays),
    InvoiceFileEnabled: values.InvoiceFileEnabled,
    InvoiceFileStorage: values.InvoiceFileStorage,
    InvoiceFileMaxSize: Math.round(
      Number(values.InvoiceFileMaxSizeMiB) * 1024 * 1024
    ),
    InvoiceFileMaxCount: Number(values.InvoiceFileMaxCount),
    InvoiceFileAllowedExts: values.InvoiceFileAllowedExts,
    InvoiceFileLocalPath: values.InvoiceFileLocalPath,
    InvoiceFileSignedURLTTL: Number(values.InvoiceFileSignedURLTTL),
    InvoiceFileOSSEndpoint: values.InvoiceFileOSSEndpoint,
    InvoiceFileOSSBucket: values.InvoiceFileOSSBucket,
    InvoiceFileOSSRegion: values.InvoiceFileOSSRegion,
    InvoiceFileOSSAccessKeyId: values.InvoiceFileOSSAccessKeyId,
    InvoiceFileOSSAccessKeySecret: values.InvoiceFileOSSAccessKeySecret,
    InvoiceFileOSSCustomDomain: values.InvoiceFileOSSCustomDomain,
    InvoiceFileS3Endpoint: values.InvoiceFileS3Endpoint,
    InvoiceFileS3Bucket: values.InvoiceFileS3Bucket,
    InvoiceFileS3Region: values.InvoiceFileS3Region,
    InvoiceFileS3AccessKeyId: values.InvoiceFileS3AccessKeyId,
    InvoiceFileS3AccessKeySecret: values.InvoiceFileS3AccessKeySecret,
    InvoiceFileS3CustomDomain: '',
    InvoiceFileCOSEndpoint: values.InvoiceFileCOSEndpoint,
    InvoiceFileCOSBucket: values.InvoiceFileCOSBucket,
    InvoiceFileCOSRegion: values.InvoiceFileCOSRegion,
    InvoiceFileCOSSecretId: values.InvoiceFileCOSSecretId,
    InvoiceFileCOSSecretKey: values.InvoiceFileCOSSecretKey,
    InvoiceFileCOSCustomDomain: values.InvoiceFileCOSCustomDomain,
  }
}

export function InvoiceSettingsSection({ defaultValues }: Props) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [values, setValues] = useState(() => initialValues(defaultValues))
  const baseline = useRef(initialValues(defaultValues))

  useEffect(() => {
    const next = initialValues(defaultValues)
    baseline.current = next
    setValues(next)
  }, [defaultValues])

  const update = (key: keyof FormValues, value: string | boolean) =>
    setValues((current) => ({ ...current, [key]: value }))
  const dirty = JSON.stringify(values) !== JSON.stringify(baseline.current)

  const saveMutation = useMutation({
    mutationFn: () => updateInvoiceSettings(buildPayload(values)),
    onSuccess: async () => {
      baseline.current = values
      await queryClient.invalidateQueries({ queryKey: ['system-options'] })
      toast.success(t('Invoice settings saved'))
    },
    onError: (error: unknown) =>
      toast.error(
        getUserFacingErrorMessage(error, 'Failed to update invoice settings')
      ),
  })
  const testMutation = useMutation({
    mutationFn: testInvoiceStorage,
    onSuccess: (result) =>
      toast.success(
        t('Invoice storage connection succeeded ({{storage}})', {
          storage: result.data?.storage || values.InvoiceFileStorage,
        })
      ),
    onError: (error: unknown) =>
      toast.error(
        getUserFacingErrorMessage(error, 'Invoice storage connection failed')
      ),
  })

  const save = () => {
    if (!dirty) {
      toast.info(t('No changes to save'))
      return
    }
    saveMutation.mutate()
  }
  const storage = (values.InvoiceFileStorage || 'local') as StorageType

  return (
    <SettingsSection title={t('Invoice Management')}>
      <div className='space-y-6'>
        <SettingsPageFormActions
          onSave={save}
          isSaving={saveMutation.isPending}
          isSaveDisabled={testMutation.isPending}
          saveLabel='Save invoice settings'
        />
        <p className='text-muted-foreground text-sm'>
          {t('Configure invoice applications, delivery, and notifications')}
        </p>

        <section className='bg-card space-y-4 rounded-xl border p-4'>
          <h4 className='text-sm font-semibold'>{t('Invoice applications')}</h4>
          <TextField
            label={t('Minimum invoice amount (CNY)')}
            value={values.InvoiceMinimumAmountYuan}
            onChange={(value) => update('InvoiceMinimumAmountYuan', value)}
            type='number'
            min='0.01'
            step='0.01'
          />
          <p className='text-muted-foreground text-xs'>
            {t(
              'Users can combine multiple paid top-up orders. The selected total must reach this amount before submission.'
            )}
          </p>
          <TextField
            label={t('Pending invoice expiry (days)')}
            value={values.InvoicePendingExpiryDays}
            onChange={(value) => update('InvoicePendingExpiryDays', value)}
            type='number'
            min='0'
            max='3650'
            step='1'
          />
          <p className='text-muted-foreground text-xs'>
            {t(
              'Pending applications expire automatically after this many days and release their orders. Set to 0 to disable automatic expiry.'
            )}
          </p>
          <TextField
            label={t('Completed invoice data retention (days)')}
            value={values.InvoiceDataRetentionDays}
            onChange={(value) => update('InvoiceDataRetentionDays', value)}
            type='number'
            min='0'
            max='36500'
            step='1'
          />
          <p className='text-muted-foreground text-xs'>
            {t(
              'Set to 0 to keep records indefinitely. A value from 30 to 36500 removes billing details and invoice files after the retention period while preserving amount, status, and order claims.'
            )}
          </p>
        </section>

        <section className='bg-card space-y-4 rounded-xl border p-4'>
          <h4 className='text-sm font-semibold'>{t('Notification')}</h4>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Invoice emails use the shared SMTP configuration under System Settings > Email.'
            )}
          </p>
          <SettingSwitch
            label={t('Notify admin about invoice applications')}
            description={t(
              'Send an email when a user submits an invoice application'
            )}
            checked={values.InvoiceApplicationNotifyAdminEnabled}
            onCheckedChange={(value) =>
              update('InvoiceApplicationNotifyAdminEnabled', value)
            }
          />
          <TextField
            label={t('Invoice admin email')}
            value={values.InvoiceAdminEmail}
            onChange={(value) => update('InvoiceAdminEmail', value)}
            type='email'
            placeholder='billing@example.com'
          />
          <p className='text-muted-foreground text-xs'>
            {t(
              'This address receives new-application notices and a reminder 24 hours before a pending application expires.'
            )}
          </p>
          <SettingSwitch
            label={t('Notify user when an invoice is issued')}
            description={t(
              'Send the notification and electronic invoice attachments through SMTP to the email bound to the user account; the invoice remains downloadable in the console.'
            )}
            checked={values.InvoiceIssuedNotifyUserEnabled}
            onCheckedChange={(value) =>
              update('InvoiceIssuedNotifyUserEnabled', value)
            }
          />
        </section>

        <section className='bg-card space-y-4 rounded-xl border p-4'>
          <h4 className='text-sm font-semibold'>{t('Invoice files')}</h4>
          <SettingSwitch
            label={t('Enable invoice file delivery')}
            description={t('Allow administrators to upload issued invoices')}
            checked={values.InvoiceFileEnabled}
            onCheckedChange={(value) => update('InvoiceFileEnabled', value)}
          />
          <div className='space-y-1.5'>
            <Label htmlFor='invoice-file-storage'>{t('File storage')}</Label>
            <Select
              value={storage}
              onValueChange={(value) =>
                value && update('InvoiceFileStorage', value)
              }
            >
              <SelectTrigger
                id='invoice-file-storage'
                className='w-full sm:w-72'
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='local'>{t('Local disk')}</SelectItem>
                <SelectItem value='oss'>{t('Alibaba Cloud OSS')}</SelectItem>
                <SelectItem value='s3'>{t('S3 compatible storage')}</SelectItem>
                <SelectItem value='cos'>{t('Tencent Cloud COS')}</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className='grid gap-3 sm:grid-cols-2'>
            <TextField
              label={t('Max size per file (MiB)')}
              value={values.InvoiceFileMaxSizeMiB}
              onChange={(value) => update('InvoiceFileMaxSizeMiB', value)}
              type='number'
              min='1'
              max='5'
              step='1'
            />
            <TextField
              label={t('Max files per invoice')}
              value={values.InvoiceFileMaxCount}
              onChange={(value) => update('InvoiceFileMaxCount', value)}
              type='number'
              min='1'
              max='20'
              step='1'
            />
            <TextField
              label={t('Allowed extensions (comma separated)')}
              value={values.InvoiceFileAllowedExts}
              onChange={(value) => update('InvoiceFileAllowedExts', value)}
              placeholder='jpg,jpeg,png,webp,pdf'
              className='sm:col-span-2'
            />
            {storage === 'local' && (
              <TextField
                label={t('Local storage path')}
                value={values.InvoiceFileLocalPath}
                onChange={(value) => update('InvoiceFileLocalPath', value)}
                placeholder='/data/invoice_files'
              />
            )}
            <TextField
              label={t('Signed URL TTL (seconds)')}
              value={values.InvoiceFileSignedURLTTL}
              onChange={(value) => update('InvoiceFileSignedURLTTL', value)}
              type='number'
              min='60'
              max='86400'
              step='1'
            />
          </div>
          {storage !== 'local' && (
            <div className='grid gap-3 rounded-lg border p-3 sm:grid-cols-2'>
              {STORAGE_FIELDS[storage].map(([key, label, secret]) => {
                let placeholder = ''
                if (secret) {
                  placeholder = t('Leave blank to keep the saved secret')
                } else if (label === 'Custom domain') {
                  placeholder = 'https://cdn.example.com'
                }
                return (
                  <TextField
                    key={key}
                    label={t(label)}
                    value={values[key]}
                    onChange={(value) => update(key, value)}
                    type={secret ? 'password' : 'text'}
                    placeholder={placeholder}
                  />
                )
              })}
            </div>
          )}
        </section>

        <div className='flex flex-wrap items-center justify-end gap-3'>
          {dirty && (
            <p className='text-muted-foreground mr-auto text-xs'>
              {t('Save the current settings before testing the connection.')}
            </p>
          )}
          <Button
            variant='outline'
            onClick={() => testMutation.mutate()}
            disabled={dirty || testMutation.isPending || saveMutation.isPending}
          >
            {t('Test storage connection')}
          </Button>
        </div>
      </div>
    </SettingsSection>
  )
}

function SettingSwitch({
  label,
  description,
  checked,
  onCheckedChange,
}: {
  label: string
  description: string
  checked: boolean
  onCheckedChange: (checked: boolean) => void
}) {
  const id = useId()
  return (
    <div className='flex items-start justify-between gap-3 rounded-lg border p-3'>
      <div className='space-y-0.5'>
        <Label htmlFor={id} className='text-sm font-medium'>
          {label}
        </Label>
        <div className='text-muted-foreground text-xs'>{description}</div>
      </div>
      <Switch id={id} checked={checked} onCheckedChange={onCheckedChange} />
    </div>
  )
}

function TextField({
  label,
  value,
  onChange,
  type = 'text',
  placeholder,
  className,
  min,
  max,
  step,
}: {
  label: string
  value: string
  onChange: (value: string) => void
  type?: string
  placeholder?: string
  className?: string
  min?: string
  max?: string
  step?: string
}) {
  const id = useId()
  return (
    <div className={`space-y-1.5 ${className ?? ''}`}>
      <Label htmlFor={id}>{label}</Label>
      <Input
        id={id}
        type={type}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
        min={min}
        max={max}
        step={step}
      />
    </div>
  )
}

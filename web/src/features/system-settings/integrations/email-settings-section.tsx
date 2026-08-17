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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Loader2, Send } from 'lucide-react'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Form } from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { getUserFacingErrorMessage } from '@/lib/user-facing-error'

import { testSMTPEmail } from '../api'
import {
  SettingsControlGroup,
  SettingsForm,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'
import {
  SmtpChannelFields,
  type SmtpChannelFieldNames,
} from './smtp-channel-fields'

const portSchema = (message: string) =>
  z.string().refine((value) => {
    const trimmed = value.trim()
    if (!trimmed) return true
    if (!/^\d+$/.test(trimmed)) return false
    const port = Number(trimmed)
    return port >= 1 && port <= 65535
  }, message)

const emailSchema = (message: string) =>
  z.string().refine((value) => {
    const trimmed = value.trim()
    if (!trimmed) return true
    return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(trimmed)
  }, message)

const createEmailSchema = (t: (key: string) => string) =>
  z
    .object({
      SMTPServer: z.string(),
      SMTPPort: portSchema(t('Port must be between 1 and 65535')),
      SMTPAccount: z.string(),
      SMTPFrom: emailSchema(t('Enter a valid email or leave blank')),
      SMTPToken: z.string(),
      SMTPSSLEnabled: z.boolean(),
      SMTPStartTLSEnabled: z.boolean(),
      SMTPInsecureSkipVerify: z.boolean(),
      SMTPForceAuthLogin: z.boolean(),
      SMTPBackupEnabled: z.boolean(),
      SMTPBackupServer: z.string(),
      SMTPBackupPort: portSchema(t('Port must be between 1 and 65535')),
      SMTPBackupAccount: z.string(),
      SMTPBackupFrom: emailSchema(t('Enter a valid email or leave blank')),
      SMTPBackupToken: z.string(),
      SMTPBackupSSLEnabled: z.boolean(),
      SMTPBackupStartTLSEnabled: z.boolean(),
      SMTPBackupInsecureSkipVerify: z.boolean(),
      SMTPBackupForceAuthLogin: z.boolean(),
    })
    .superRefine((values, context) => {
      const hasBackupConfiguration = Boolean(
        values.SMTPBackupServer.trim() ||
        values.SMTPBackupAccount.trim() ||
        values.SMTPBackupFrom.trim() ||
        values.SMTPBackupToken.trim()
      )
      if (!hasBackupConfiguration) return
      if (!values.SMTPBackupServer.trim()) {
        context.addIssue({
          code: 'custom',
          path: ['SMTPBackupServer'],
          message: t('Backup SMTP host is required when failover is enabled'),
        })
      }
      if (!values.SMTPBackupFrom.trim() && !values.SMTPBackupAccount.trim()) {
        context.addIssue({
          code: 'custom',
          path: ['SMTPBackupFrom'],
          message: t(
            'Backup sender address or username is required when failover is enabled'
          ),
        })
      }
    })

export type EmailFormValues = z.infer<ReturnType<typeof createEmailSchema>>

type EmailSettingsSectionProps = {
  defaultValues: EmailFormValues
}

const PRIMARY_FIELDS = {
  server: 'SMTPServer',
  port: 'SMTPPort',
  account: 'SMTPAccount',
  from: 'SMTPFrom',
  token: 'SMTPToken',
  sslEnabled: 'SMTPSSLEnabled',
  startTLSEnabled: 'SMTPStartTLSEnabled',
  insecureSkipVerify: 'SMTPInsecureSkipVerify',
  forceAuthLogin: 'SMTPForceAuthLogin',
} as const satisfies SmtpChannelFieldNames

const BACKUP_FIELDS = {
  server: 'SMTPBackupServer',
  port: 'SMTPBackupPort',
  account: 'SMTPBackupAccount',
  from: 'SMTPBackupFrom',
  token: 'SMTPBackupToken',
  sslEnabled: 'SMTPBackupSSLEnabled',
  startTLSEnabled: 'SMTPBackupStartTLSEnabled',
  insecureSkipVerify: 'SMTPBackupInsecureSkipVerify',
  forceAuthLogin: 'SMTPBackupForceAuthLogin',
} as const satisfies SmtpChannelFieldNames

const SMTP_OPTION_KEYS = [
  'SMTPServer',
  'SMTPPort',
  'SMTPAccount',
  'SMTPFrom',
  'SMTPToken',
  'SMTPSSLEnabled',
  'SMTPStartTLSEnabled',
  'SMTPInsecureSkipVerify',
  'SMTPForceAuthLogin',
  'SMTPBackupServer',
  'SMTPBackupPort',
  'SMTPBackupAccount',
  'SMTPBackupFrom',
  'SMTPBackupToken',
  'SMTPBackupSSLEnabled',
  'SMTPBackupStartTLSEnabled',
  'SMTPBackupInsecureSkipVerify',
  'SMTPBackupForceAuthLogin',
] as const satisfies ReadonlyArray<keyof EmailFormValues>

const SMTP_BACKUP_CONFIGURATION_KEYS = [
  'SMTPBackupServer',
  'SMTPBackupPort',
  'SMTPBackupAccount',
  'SMTPBackupFrom',
  'SMTPBackupToken',
  'SMTPBackupSSLEnabled',
  'SMTPBackupStartTLSEnabled',
  'SMTPBackupInsecureSkipVerify',
  'SMTPBackupForceAuthLogin',
] as const satisfies ReadonlyArray<keyof EmailFormValues>

type SMTPChannel = 'primary' | 'backup'

function sanitize(values: EmailFormValues): EmailFormValues {
  return {
    ...values,
    SMTPServer: values.SMTPServer.trim(),
    SMTPPort: values.SMTPPort.trim(),
    SMTPAccount: values.SMTPAccount.trim(),
    SMTPFrom: values.SMTPFrom.trim(),
    SMTPToken: values.SMTPToken.trim(),
    SMTPStartTLSEnabled: !values.SMTPSSLEnabled && values.SMTPStartTLSEnabled,
    SMTPBackupServer: values.SMTPBackupServer.trim(),
    SMTPBackupPort: values.SMTPBackupPort.trim(),
    SMTPBackupAccount: values.SMTPBackupAccount.trim(),
    SMTPBackupFrom: values.SMTPBackupFrom.trim(),
    SMTPBackupToken: values.SMTPBackupToken.trim(),
    SMTPBackupStartTLSEnabled:
      !values.SMTPBackupSSLEnabled && values.SMTPBackupStartTLSEnabled,
  }
}

export function EmailSettingsSection({
  defaultValues,
}: EmailSettingsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const queryClient = useQueryClient()
  const [testRecipient, setTestRecipient] = useState('')
  const [activeChannel, setActiveChannel] = useState<SMTPChannel>('primary')
  const form = useForm<EmailFormValues>({
    resolver: zodResolver(createEmailSchema(t)),
    defaultValues,
  })

  useResetForm(form, defaultValues)

  const testMutation = useMutation({
    mutationFn: (channel: SMTPChannel) =>
      testSMTPEmail(testRecipient.trim(), channel),
    onSuccess: (result, channel) => {
      if (channel === 'backup') {
        form.reset({
          ...form.getValues(),
          SMTPBackupEnabled: true,
        })
        void queryClient.invalidateQueries({ queryKey: ['system-options'] })
      }
      const channelLabel =
        result.data?.channel === 'backup'
          ? t('Backup channel')
          : t('Primary channel')
      toast.success(
        t('Test email sent to {{recipient}} via {{channel}}', {
          recipient: result.data?.recipient || testRecipient,
          channel: channelLabel,
        })
      )
    },
    onError: (error: unknown) =>
      toast.error(getUserFacingErrorMessage(error, 'SMTP test email failed')),
  })

  const persistSettings = async (
    values: EmailFormValues,
    showNoChanges: boolean
  ) => {
    const next = sanitize(values)
    const initial = sanitize(defaultValues)
    const updates: Array<{ key: string; value: string | boolean }> = []

    for (const key of SMTP_OPTION_KEYS) {
      const value = next[key]
      if ((key === 'SMTPToken' || key === 'SMTPBackupToken') && !value) {
        continue
      }
      if (value !== initial[key]) {
        updates.push({ key, value })
      }
    }

    if (updates.length === 0) {
      if (showNoChanges) toast.info(t('No changes to save'))
      return true
    }
    const backupConfigurationChanged = updates.some((update) =>
      SMTP_BACKUP_CONFIGURATION_KEYS.includes(
        update.key as (typeof SMTP_BACKUP_CONFIGURATION_KEYS)[number]
      )
    )
    for (const update of updates) {
      const response = await updateOption.mutateAsync(update)
      if (!response.success) return false
    }
    form.reset({
      ...next,
      SMTPToken: '',
      SMTPBackupToken: '',
      SMTPBackupEnabled: backupConfigurationChanged
        ? false
        : next.SMTPBackupEnabled,
    })
    return true
  }

  const onSubmit = async (values: EmailFormValues) => {
    await persistSettings(values, true)
  }

  const sendTest = form.handleSubmit(async (values) => {
    try {
      if (!(await persistSettings(values, false))) return
      await testMutation.mutateAsync(activeChannel)
    } catch {
      // The update and test mutations already present their own user-facing errors.
    }
  })

  const backupConfigurationDirty = SMTP_BACKUP_CONFIGURATION_KEYS.some((key) =>
    Boolean(form.formState.dirtyFields[key])
  )
  const backupEnabled =
    form.watch('SMTPBackupEnabled') && !backupConfigurationDirty
  const operationPending = testMutation.isPending || updateOption.isPending

  return (
    <SettingsSection title={t('SMTP Email')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)} autoComplete='off'>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save SMTP settings'
          />

          <Tabs
            value={activeChannel}
            onValueChange={(value) => setActiveChannel(value as SMTPChannel)}
            className='space-y-5'
          >
            <TabsList className='grid w-full max-w-md grid-cols-2'>
              <TabsTrigger value='primary'>{t('Primary channel')}</TabsTrigger>
              <TabsTrigger value='backup'>{t('Backup channel')}</TabsTrigger>
            </TabsList>

            <TabsContent value='primary' className='pt-1'>
              <SmtpChannelFields form={form} names={PRIMARY_FIELDS} />
            </TabsContent>

            <TabsContent value='backup' className='space-y-6 pt-1'>
              <SettingsControlGroup className='flex flex-col justify-between gap-3 space-y-0 sm:flex-row sm:items-center'>
                <div className='min-w-0 space-y-1'>
                  <p className='text-sm font-medium'>
                    {t('Backup SMTP activation')}
                  </p>
                  <p className='text-muted-foreground text-xs'>
                    {t(
                      'A successful backup test automatically enables failover. Changing backup settings requires another test.'
                    )}
                  </p>
                </div>
                <Badge
                  variant='outline'
                  className={
                    backupEnabled
                      ? 'border-emerald-500/40 bg-emerald-500/10 text-emerald-600'
                      : 'border-amber-500/40 bg-amber-500/10 text-amber-600'
                  }
                >
                  {backupEnabled ? t('Enabled') : t('Pending test')}
                </Badge>
              </SettingsControlGroup>
              <SmtpChannelFields form={form} names={BACKUP_FIELDS} />
            </TabsContent>
          </Tabs>

          <SettingsControlGroup className='space-y-4'>
            <div className='space-y-1'>
              <Label htmlFor='smtp-test-recipient'>{t('Test SMTP')}</Label>
              <p className='text-muted-foreground text-xs'>
                {t(
                  'The test saves current settings first and sends through the selected channel'
                )}
              </p>
            </div>
            <div className='flex flex-col gap-3 sm:flex-row'>
              <Input
                id='smtp-test-recipient'
                type='email'
                value={testRecipient}
                onChange={(event) => setTestRecipient(event.target.value)}
                placeholder={t(
                  'Leave blank to use the current administrator email'
                )}
                disabled={operationPending}
              />
              <Button
                type='button'
                className='sm:shrink-0'
                onClick={sendTest}
                disabled={operationPending}
              >
                {testMutation.isPending ? (
                  <Loader2 className='animate-spin' />
                ) : (
                  <Send />
                )}
                {activeChannel === 'backup'
                  ? t('Test and enable backup channel')
                  : t('Send test email')}
              </Button>
            </div>
          </SettingsControlGroup>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}

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
import { EmailReceiptSettings } from './email-receipt-settings'
import { MarketingEmailAccounts } from './marketing-email-accounts'
import {
  SmtpChannelFields,
  type SmtpChannelFieldNames,
} from './smtp-channel-fields'
import {
  SMTPProfileStatusCard,
  type SMTPProfileState,
} from './smtp-profile-status-card'

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
      SMTPSecurityEnabled: z.boolean(),
      SMTPSecurityServer: z.string(),
      SMTPSecurityPort: portSchema(t('Port must be between 1 and 65535')),
      SMTPSecurityAccount: z.string(),
      SMTPSecurityFrom: emailSchema(t('Enter a valid email or leave blank')),
      SMTPSecurityToken: z.string(),
      SMTPSecuritySSLEnabled: z.boolean(),
      SMTPSecurityStartTLSEnabled: z.boolean(),
      SMTPSecurityInsecureSkipVerify: z.boolean(),
      SMTPSecurityForceAuthLogin: z.boolean(),
      SMTPMarketingEnabled: z.boolean(),
      SMTPMarketingServer: z.string(),
      SMTPMarketingPort: portSchema(t('Port must be between 1 and 65535')),
      SMTPMarketingAccount: z.string(),
      SMTPMarketingFrom: emailSchema(t('Enter a valid email or leave blank')),
      SMTPMarketingToken: z.string(),
      SMTPMarketingSSLEnabled: z.boolean(),
      SMTPMarketingStartTLSEnabled: z.boolean(),
      SMTPMarketingInsecureSkipVerify: z.boolean(),
      SMTPMarketingForceAuthLogin: z.boolean(),
    })
    .superRefine((values, context) => {
      const hasBackupConfiguration = Boolean(
        values.SMTPBackupServer.trim() ||
        values.SMTPBackupAccount.trim() ||
        values.SMTPBackupFrom.trim() ||
        values.SMTPBackupToken.trim()
      )
      if (hasBackupConfiguration && !values.SMTPBackupServer.trim()) {
        context.addIssue({
          code: 'custom',
          path: ['SMTPBackupServer'],
          message: t('Backup SMTP host is required when failover is enabled'),
        })
      }
      if (
        hasBackupConfiguration &&
        !values.SMTPBackupFrom.trim() &&
        !values.SMTPBackupAccount.trim()
      ) {
        context.addIssue({
          code: 'custom',
          path: ['SMTPBackupFrom'],
          message: t(
            'Backup sender address or username is required when failover is enabled'
          ),
        })
      }

      const profiles = [
        {
          server: values.SMTPSecurityServer,
          account: values.SMTPSecurityAccount,
          from: values.SMTPSecurityFrom,
          token: values.SMTPSecurityToken,
          serverPath: 'SMTPSecurityServer' as const,
          fromPath: 'SMTPSecurityFrom' as const,
        },
        {
          server: values.SMTPMarketingServer,
          account: values.SMTPMarketingAccount,
          from: values.SMTPMarketingFrom,
          token: values.SMTPMarketingToken,
          serverPath: 'SMTPMarketingServer' as const,
          fromPath: 'SMTPMarketingFrom' as const,
        },
      ]
      for (const profile of profiles) {
        const configured = Boolean(
          profile.server.trim() ||
          profile.account.trim() ||
          profile.from.trim() ||
          profile.token.trim()
        )
        if (!configured) continue
        if (!profile.server.trim()) {
          context.addIssue({
            code: 'custom',
            path: [profile.serverPath],
            message: t('SMTP host is required when a profile is configured'),
          })
        }
        if (!profile.from.trim() && !profile.account.trim()) {
          context.addIssue({
            code: 'custom',
            path: [profile.fromPath],
            message: t(
              'Sender address or username is required when a profile is configured'
            ),
          })
        }
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

const SECURITY_FIELDS = {
  server: 'SMTPSecurityServer',
  port: 'SMTPSecurityPort',
  account: 'SMTPSecurityAccount',
  from: 'SMTPSecurityFrom',
  token: 'SMTPSecurityToken',
  sslEnabled: 'SMTPSecuritySSLEnabled',
  startTLSEnabled: 'SMTPSecurityStartTLSEnabled',
  insecureSkipVerify: 'SMTPSecurityInsecureSkipVerify',
  forceAuthLogin: 'SMTPSecurityForceAuthLogin',
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
  'SMTPSecurityServer',
  'SMTPSecurityPort',
  'SMTPSecurityAccount',
  'SMTPSecurityFrom',
  'SMTPSecurityToken',
  'SMTPSecuritySSLEnabled',
  'SMTPSecurityStartTLSEnabled',
  'SMTPSecurityInsecureSkipVerify',
  'SMTPSecurityForceAuthLogin',
  'SMTPMarketingServer',
  'SMTPMarketingPort',
  'SMTPMarketingAccount',
  'SMTPMarketingFrom',
  'SMTPMarketingToken',
  'SMTPMarketingSSLEnabled',
  'SMTPMarketingStartTLSEnabled',
  'SMTPMarketingInsecureSkipVerify',
  'SMTPMarketingForceAuthLogin',
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

type SMTPChannel = 'security' | 'primary' | 'marketing' | 'backup'
type SMTPSettingsTab =
  | 'security'
  | 'primary'
  | 'backup'
  | 'marketing'
  | 'receipt'

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
    SMTPSecurityServer: values.SMTPSecurityServer.trim(),
    SMTPSecurityPort: values.SMTPSecurityPort.trim(),
    SMTPSecurityAccount: values.SMTPSecurityAccount.trim(),
    SMTPSecurityFrom: values.SMTPSecurityFrom.trim(),
    SMTPSecurityToken: values.SMTPSecurityToken.trim(),
    SMTPSecurityStartTLSEnabled:
      !values.SMTPSecuritySSLEnabled && values.SMTPSecurityStartTLSEnabled,
    SMTPMarketingServer: values.SMTPMarketingServer.trim(),
    SMTPMarketingPort: values.SMTPMarketingPort.trim(),
    SMTPMarketingAccount: values.SMTPMarketingAccount.trim(),
    SMTPMarketingFrom: values.SMTPMarketingFrom.trim(),
    SMTPMarketingToken: values.SMTPMarketingToken.trim(),
    SMTPMarketingStartTLSEnabled:
      !values.SMTPMarketingSSLEnabled && values.SMTPMarketingStartTLSEnabled,
  }
}

export function EmailSettingsSection({
  defaultValues,
}: EmailSettingsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const queryClient = useQueryClient()
  const [testRecipient, setTestRecipient] = useState('')
  const [activeChannel, setActiveChannel] =
    useState<SMTPSettingsTab>('security')
  const form = useForm<EmailFormValues>({
    resolver: zodResolver(createEmailSchema(t)),
    defaultValues,
  })

  useResetForm(form, defaultValues)

  const testMutation = useMutation({
    mutationFn: (channel: SMTPChannel) =>
      testSMTPEmail(testRecipient.trim(), channel),
    onSuccess: (result, channel) => {
      if (channel !== 'primary') {
        const enabledUpdates: Partial<EmailFormValues> = {}
        if (channel === 'backup') enabledUpdates.SMTPBackupEnabled = true
        if (channel === 'security') enabledUpdates.SMTPSecurityEnabled = true
        if (channel === 'marketing') enabledUpdates.SMTPMarketingEnabled = true
        form.reset({ ...form.getValues(), ...enabledUpdates })
        void queryClient.invalidateQueries({ queryKey: ['system-options'] })
      }
      const channelLabels: Record<SMTPChannel, string> = {
        security: t('Security mail'),
        primary: t('Notification mail'),
        marketing: t('Marketing mail'),
        backup: t('Backup channel'),
      }
      const deliveredChannel = result.data?.channel as SMTPChannel | undefined
      const channelLabel = deliveredChannel
        ? channelLabels[deliveredChannel]
        : channelLabels[channel]
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
      if (key.endsWith('Token') && !value) {
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
    const securityConfigurationChanged = updates.some((update) =>
      update.key.startsWith('SMTPSecurity')
    )
    const marketingConfigurationChanged = updates.some((update) =>
      update.key.startsWith('SMTPMarketing')
    )
    for (const update of updates) {
      const response = await updateOption.mutateAsync(update)
      if (!response.success) return false
    }
    form.reset({
      ...next,
      SMTPToken: '',
      SMTPBackupToken: '',
      SMTPSecurityToken: '',
      SMTPMarketingToken: '',
      SMTPBackupEnabled: backupConfigurationChanged
        ? false
        : next.SMTPBackupEnabled,
      SMTPSecurityEnabled: securityConfigurationChanged
        ? false
        : next.SMTPSecurityEnabled,
      SMTPMarketingEnabled: marketingConfigurationChanged
        ? false
        : next.SMTPMarketingEnabled,
    })
    return true
  }

  const onSubmit = async (values: EmailFormValues) => {
    await persistSettings(values, true)
  }

  const sendTest = form.handleSubmit(async (values) => {
    try {
      if (!(await persistSettings(values, false))) return
      if (
        activeChannel === 'security' ||
        activeChannel === 'primary' ||
        activeChannel === 'backup'
      ) {
        await testMutation.mutateAsync(activeChannel)
      }
    } catch {
      // The update and test mutations already present their own user-facing errors.
    }
  })

  const backupConfigurationDirty = SMTP_BACKUP_CONFIGURATION_KEYS.some((key) =>
    Boolean(form.formState.dirtyFields[key])
  )
  const backupEnabled =
    form.watch('SMTPBackupEnabled') && !backupConfigurationDirty
  const securityConfigurationDirty = SMTP_OPTION_KEYS.some(
    (key) =>
      key.startsWith('SMTPSecurity') && Boolean(form.formState.dirtyFields[key])
  )
  const securityEnabled =
    form.watch('SMTPSecurityEnabled') && !securityConfigurationDirty
  const primaryConfigured = Boolean(
    form.watch('SMTPServer') &&
    (form.watch('SMTPFrom') || form.watch('SMTPAccount'))
  )
  const securityConfigured = Boolean(
    form.watch('SMTPSecurityServer') &&
    (form.watch('SMTPSecurityFrom') || form.watch('SMTPSecurityAccount'))
  )
  const backupConfigured = Boolean(
    form.watch('SMTPBackupServer') &&
    (form.watch('SMTPBackupFrom') || form.watch('SMTPBackupAccount'))
  )
  let securityState: SMTPProfileState = 'disabled'
  if (securityConfigured) {
    securityState = 'pending'
  }
  if (securityEnabled) {
    securityState = 'enabled'
  }
  let backupState: SMTPProfileState = 'disabled'
  if (backupConfigured) {
    backupState = 'pending'
  }
  if (backupEnabled) {
    backupState = 'enabled'
  }
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
            onValueChange={(value) =>
              setActiveChannel(value as SMTPSettingsTab)
            }
            className='space-y-5'
          >
            <TabsList className='grid w-full max-w-5xl grid-cols-2 sm:grid-cols-5'>
              <TabsTrigger value='security'>{t('Security mail')}</TabsTrigger>
              <TabsTrigger value='primary'>
                {t('Notification mail')}
              </TabsTrigger>
              <TabsTrigger value='backup'>{t('Backup channel')}</TabsTrigger>
              <TabsTrigger value='marketing'>{t('Marketing mail')}</TabsTrigger>
              <TabsTrigger value='receipt'>
                {t('Receipt interface')}
              </TabsTrigger>
            </TabsList>

            <TabsContent value='security' className='space-y-6 pt-1'>
              <SMTPProfileStatusCard
                title={t('Verification and security email configuration')}
                description={t(
                  'Registration codes, email binding and password reset use this sender. A successful test activates the profile.'
                )}
                state={securityState}
              />
              <SmtpChannelFields form={form} names={SECURITY_FIELDS} />
            </TabsContent>

            <TabsContent value='primary' className='space-y-6 pt-1'>
              <SMTPProfileStatusCard
                title={t('Notification and business email configuration')}
                description={t(
                  'Quota warnings, invoices, affiliate notices and operational alerts use this sender.'
                )}
                state={primaryConfigured ? 'enabled' : 'disabled'}
                status={
                  primaryConfigured ? t('Configured') : t('Not configured')
                }
              />
              <SmtpChannelFields form={form} names={PRIMARY_FIELDS} />
            </TabsContent>

            <TabsContent value='backup' className='space-y-6 pt-1'>
              <SMTPProfileStatusCard
                title={t('Backup SMTP activation status')}
                description={t(
                  'A successful backup test automatically enables failover. Changing backup settings requires another test.'
                )}
                state={backupState}
              />
              <SmtpChannelFields form={form} names={BACKUP_FIELDS} />
            </TabsContent>

            <TabsContent value='marketing' className='pt-1'>
              <MarketingEmailAccounts />
            </TabsContent>

            <TabsContent value='receipt' className='pt-1'>
              <EmailReceiptSettings />
            </TabsContent>
          </Tabs>

          {activeChannel !== 'marketing' && activeChannel !== 'receipt' ? (
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
                  {activeChannel === 'primary'
                    ? t('Send test email')
                    : t('Test and enable selected profile')}
                </Button>
              </div>
            </SettingsControlGroup>
          ) : null}
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}

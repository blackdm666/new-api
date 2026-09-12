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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Pencil, Plus, Send, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { formatTimestampToDate } from '@/lib/format'
import { getUserFacingErrorMessage } from '@/lib/user-facing-error'

import {
  createMarketingEmailSenderAccount,
  deleteMarketingEmailSenderAccount,
  getEmailReceiptEndpoint,
  listMarketingEmailSenderAccounts,
  setMarketingEmailSenderAccountEnabled,
  testMarketingEmailSenderAccount,
  updateMarketingEmailSenderAccount,
} from '../api'
import { SettingsControlGroup } from '../components/settings-form-layout'
import type {
  MarketingEmailSenderAccount,
  MarketingEmailSenderAccountInput,
} from '../types'
import { MarketingEmailAccountEditor } from './marketing-email-account-editor'
import {
  SMTPProfileStatusCard,
  type SMTPProfileState,
} from './smtp-profile-status-card'

const EMPTY_ACCOUNT: MarketingEmailSenderAccountInput = {
  name: '',
  provider: 'aliyun_eventbridge',
  server: 'smtpdm.aliyun.com',
  port: 465,
  account: '',
  from: '',
  token: '',
  ssl_enabled: true,
  starttls_enabled: false,
  insecure_skip_verify: false,
  force_auth_login: false,
  weight: 1,
  rate_limit_per_minute: 20,
}

export function MarketingEmailAccounts() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [editingId, setEditingId] = useState<number | null>(null)
  const [editorValues, setEditorValues] =
    useState<MarketingEmailSenderAccountInput | null>(null)
  const [testRecipient, setTestRecipient] = useState('')
  const query = useQuery({
    queryKey: ['marketing-email-accounts'],
    queryFn: listMarketingEmailSenderAccounts,
    refetchInterval: 15_000,
  })
  const receiptQuery = useQuery({
    queryKey: ['email-receipt-endpoint'],
    queryFn: getEmailReceiptEndpoint,
    refetchInterval: 15_000,
  })
  const accounts = query.data?.data ?? []
  const now = Date.now() / 1000
  const enabledCount = accounts.filter(
    (account) => account.enabled && account.disabled_until <= now
  ).length

  const saveMutation = useMutation({
    mutationFn: async (values: MarketingEmailSenderAccountInput) => {
      if (editingId !== null) {
        return updateMarketingEmailSenderAccount(editingId, values)
      }
      return createMarketingEmailSenderAccount(values)
    },
    onSuccess: async () => {
      setEditingId(null)
      setEditorValues(null)
      toast.success(t('Marketing sender account saved'))
      await queryClient.invalidateQueries({
        queryKey: ['marketing-email-accounts'],
      })
    },
    onError: (error: unknown) => toast.error(getUserFacingErrorMessage(error)),
  })
  const deleteMutation = useMutation({
    mutationFn: deleteMarketingEmailSenderAccount,
    onSuccess: async () => {
      toast.success(t('Marketing sender account deleted'))
      await queryClient.invalidateQueries({
        queryKey: ['marketing-email-accounts'],
      })
    },
    onError: (error: unknown) => toast.error(getUserFacingErrorMessage(error)),
  })
  const testMutation = useMutation({
    mutationFn: (id: number) =>
      testMarketingEmailSenderAccount(id, testRecipient.trim()),
    onSuccess: async () => {
      toast.success(
        t('SMTP accepted the test email; waiting for EventBridge receipt')
      )
      await queryClient.invalidateQueries({
        queryKey: ['marketing-email-accounts'],
      })
    },
    onError: (error: unknown) => toast.error(getUserFacingErrorMessage(error)),
  })
  const statusMutation = useMutation({
    mutationFn: (request: { id: number; enabled: boolean }) =>
      setMarketingEmailSenderAccountEnabled(request.id, request.enabled),
    onSuccess: async () => {
      toast.success(t('Marketing sender account status updated'))
      await queryClient.invalidateQueries({
        queryKey: ['marketing-email-accounts'],
      })
    },
    onError: (error: unknown) => toast.error(getUserFacingErrorMessage(error)),
  })

  let profileState: SMTPProfileState = 'disabled'
  let profileStatus: string | undefined
  if (enabledCount > 0) {
    if (
      receiptQuery.data?.data.enabled &&
      receiptQuery.data.data.last_verified_time > 0
    ) {
      profileState = 'enabled'
    } else if (receiptQuery.data?.data.enabled) {
      profileState = 'pending'
      profileStatus = t('Receipt interface pending verification')
    } else if (receiptQuery.data) {
      profileState = 'error'
      profileStatus = t('Receipt interface disabled')
    } else {
      profileState = 'pending'
    }
  } else if (accounts.some((account) => account.health_status === 'degraded')) {
    profileState = 'error'
  } else if (accounts.length > 0) {
    profileState = 'pending'
  }

  const edit = (account: MarketingEmailSenderAccount) => {
    setEditingId(account.id)
    setEditorValues({
      name: account.name,
      provider: account.provider,
      server: account.server,
      port: account.port,
      account: account.account,
      from: account.from,
      token: '',
      ssl_enabled: account.ssl_enabled,
      starttls_enabled: account.starttls_enabled,
      insecure_skip_verify: account.insecure_skip_verify,
      force_auth_login: account.force_auth_login,
      weight: account.weight,
      rate_limit_per_minute: account.rate_limit_per_minute,
    })
  }

  return (
    <div className='space-y-6'>
      <SMTPProfileStatusCard
        title={t('Marketing email accounts')}
        description={t(
          'Campaigns use verified Alibaba Cloud Direct Mail accounts with weighted rotation.'
        )}
        state={profileState}
        status={
          profileStatus ??
          t('{{enabled}} / {{total}} enabled', {
            enabled: enabledCount,
            total: accounts.length,
          })
        }
      />

      <div className='flex flex-wrap items-center justify-between gap-3'>
        <div className='space-y-1'>
          <Label htmlFor='marketing-account-test-recipient'>
            {t('Test recipient')}
          </Label>
          <Input
            id='marketing-account-test-recipient'
            className='w-80 max-w-full'
            type='email'
            value={testRecipient}
            onChange={(event) => setTestRecipient(event.target.value)}
            placeholder={t(
              'Leave blank to use the current administrator email'
            )}
          />
        </div>
        <Button
          type='button'
          onClick={() => {
            setEditingId(null)
            setEditorValues({ ...EMPTY_ACCOUNT })
          }}
        >
          <Plus className='size-4' />
          {t('Add marketing account')}
        </Button>
      </div>

      {editorValues ? (
        <MarketingEmailAccountEditor
          key={editingId ?? 'new'}
          initialValues={editorValues}
          editing={editingId !== null}
          saving={saveMutation.isPending}
          onCancel={() => {
            setEditingId(null)
            setEditorValues(null)
          }}
          onSave={(values) => saveMutation.mutate(values)}
        />
      ) : null}

      <div className='grid gap-4 lg:grid-cols-2'>
        {accounts.map((account) => (
          <SettingsControlGroup key={account.id} className='space-y-4'>
            <div className='flex items-start justify-between gap-3'>
              <div className='min-w-0'>
                <p className='font-medium'>{account.name}</p>
                <p className='text-muted-foreground truncate text-xs'>
                  {account.from}
                </p>
              </div>
              <div className='flex items-center gap-2'>
                <AccountStatus account={account} />
                <Switch
                  aria-label={t('Enable marketing account {{name}}', {
                    name: account.name,
                  })}
                  checked={account.enabled}
                  disabled={
                    statusMutation.isPending ||
                    (!account.enabled &&
                      (account.receipt_verified_time <= 0 ||
                        account.disabled_until > now ||
                        account.health_status === 'degraded'))
                  }
                  onCheckedChange={(enabled) =>
                    statusMutation.mutate({ id: account.id, enabled })
                  }
                />
              </div>
            </div>
            <div className='grid gap-3 text-sm sm:grid-cols-3'>
              <AccountFact label={t('Weight')} value={String(account.weight)} />
              <AccountFact
                label={t('RPM limit')}
                value={String(account.rate_limit_per_minute)}
              />
              <AccountFact
                label={t('Last receipt')}
                value={
                  account.receipt_verified_time
                    ? formatTimestampToDate(account.receipt_verified_time)
                    : '-'
                }
              />
            </div>
            {account.last_error ? (
              <p className='text-xs text-amber-600'>{account.last_error}</p>
            ) : null}
            <div className='flex flex-wrap justify-end gap-2'>
              <Button
                type='button'
                variant='outline'
                onClick={() => edit(account)}
              >
                <Pencil className='size-4' />
                {t('Edit')}
              </Button>
              <Button
                type='button'
                variant='outline'
                disabled={testMutation.isPending}
                onClick={() => testMutation.mutate(account.id)}
              >
                <Send className='size-4' />
                {t('Test receipt')}
              </Button>
              <Button
                type='button'
                variant='destructive'
                disabled={deleteMutation.isPending}
                onClick={() => {
                  if (
                    window.confirm(t('Delete this marketing sender account?'))
                  ) {
                    deleteMutation.mutate(account.id)
                  }
                }}
              >
                <Trash2 className='size-4' />
                {t('Delete')}
              </Button>
            </div>
          </SettingsControlGroup>
        ))}
      </div>
    </div>
  )
}

function AccountStatus(props: { account: MarketingEmailSenderAccount }) {
  const { t } = useTranslation()
  let label = t('Pending test')
  let className = 'border-amber-500/40 bg-amber-500/10 text-amber-600'
  if (props.account.disabled_until > Date.now() / 1000) {
    label = t('Temporarily throttled')
    className = 'border-red-500/40 bg-red-500/10 text-red-600'
  } else if (props.account.enabled) {
    label = t('Enabled')
    className = 'border-emerald-500/40 bg-emerald-500/10 text-emerald-600'
  } else if (props.account.health_status === 'degraded') {
    label = t('Degraded')
    className = 'border-red-500/40 bg-red-500/10 text-red-600'
  } else if (props.account.health_status === 'disabled') {
    label = t('Disabled')
    className = 'border-zinc-500/40 bg-zinc-500/10 text-zinc-500'
  }
  return <Badge className={className}>{label}</Badge>
}

function AccountFact(props: { label: string; value: string }) {
  return (
    <div>
      <p className='text-muted-foreground text-xs'>{props.label}</p>
      <p className='font-medium'>{props.value}</p>
    </div>
  )
}

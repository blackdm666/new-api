/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import {
  Copy,
  Eye,
  EyeOff,
  Loader2,
  Plus,
  TestTube2,
  Trash2,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'

import { testChannelBalanceQuery } from '../../api'
import {
  BALANCE_QUERY_AUTH_OPTIONS,
  BALANCE_QUERY_METRIC_OPTIONS,
  BALANCE_QUERY_MODE_OPTIONS,
  BALANCE_QUERY_UNIT_OPTIONS,
  buildBalanceQueryPreviewURL,
  createBalanceQueryConfig,
  getEffectiveBalanceQueryPath,
  getGCPTrialBillingTable,
  normalizeBalanceQueryConfig,
  validateBalanceQueryConfig,
} from '../../lib/balance-query'
import type {
  AdvancedCustomAuthType,
  ChannelBalanceMetricKind,
  ChannelBalanceQueryConfig,
  ChannelBalanceQueryMode,
  ChannelBalanceUnit,
} from '../../types'

type BalanceQueryEditorDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  value: ChannelBalanceQueryConfig
  channelType: number
  baseURL: string
  channelId?: number
  savedToken?: string | null
  canRevealSavedToken?: boolean
  savedTokenLoading?: boolean
  onRevealSavedToken?: () => Promise<void>
  onCopySavedToken?: () => Promise<void>
  onSave: (config: ChannelBalanceQueryConfig) => void
}

function formatLocalDateTime(timestamp: number | undefined): string {
  if (!timestamp) return ''
  const date = new Date(timestamp * 1000)
  const localDate = new Date(date.getTime() - date.getTimezoneOffset() * 60_000)
  return localDate.toISOString().slice(0, 16)
}

export function BalanceQueryEditorDialog(props: BalanceQueryEditorDialogProps) {
  const { t } = useTranslation()
  const { copyToClipboard } = useCopyToClipboard()
  const [tokenVisible, setTokenVisible] = useState(false)
  const [tokenEdited, setTokenEdited] = useState(false)
  const [testLoading, setTestLoading] = useState(false)
  const [testResult, setTestResult] = useState<{
    success: boolean
    message: string
    details?: string
  } | null>(null)
  const [draft, setDraft] = useState<ChannelBalanceQueryConfig>(() =>
    normalizeBalanceQueryConfig(props.value)
  )

  const mode = draft.mode || 'auto'
  const usesAccountToken =
    mode === 'new_api' ||
    mode === 'one_api' ||
    (mode === 'auto' && props.channelType === 60)
  const effectivePath = getEffectiveBalanceQueryPath(draft, props.channelType)
  const previewURL =
    mode === 'custom' && draft.url?.trim()
      ? draft.url.trim()
      : buildBalanceQueryPreviewURL(props.baseURL, effectivePath)
  const validationError = validateBalanceQueryConfig(
    mode === 'auto' && props.channelType === 60
      ? { ...draft, mode: 'new_api' }
      : draft
  )
  const selectedMode = BALANCE_QUERY_MODE_OPTIONS.find(
    (option) => option.value === mode
  )
  const authType = draft.auth?.type || 'header'
  const replacementToken = draft.auth?.value || ''
  const displayedToken = tokenEdited
    ? replacementToken
    : replacementToken || props.savedToken || ''
  let tokenVisibilityIcon = <Eye className='h-4 w-4' />
  if (props.savedTokenLoading) {
    tokenVisibilityIcon = <Loader2 className='h-4 w-4 animate-spin' />
  } else if (tokenVisible) {
    tokenVisibilityIcon = <EyeOff className='h-4 w-4' />
  }
  const gcpTrialTable = getGCPTrialBillingTable(draft)

  const updateGCPTrial = (
    field:
      | 'billing_account_id'
      | 'query_project_id'
      | 'dataset_id'
      | 'credential_channel_id'
      | 'total_amount'
      | 'baseline_used'
      | 'baseline_at',
    value: string | number
  ) => {
    setDraft((current) => ({
      ...current,
      gcp_trial: {
        ...current.gcp_trial,
        [field]: value,
      },
    }))
  }

  const modeItems = useMemo(
    () =>
      BALANCE_QUERY_MODE_OPTIONS.map((option) => ({
        value: option.value,
        label: t(option.label),
      })),
    [t]
  )

  const updateResponsePath = (
    field:
      | 'remaining_path'
      | 'total_path'
      | 'used_path'
      | 'currency_path'
      | 'active_path'
      | 'unlimited_path'
      | 'success_path'
      | 'success_value',
    value: string
  ) => {
    setDraft((current) => ({
      ...current,
      response: {
        ...current.response,
        [field]: value,
      },
    }))
  }

  const handleModeChange = (value: string | null) => {
    const nextMode = (value || 'auto') as ChannelBalanceQueryMode
    setDraft((current) => {
      if (nextMode === current.mode) return current
      return createBalanceQueryConfig(nextMode)
    })
  }

  const handleAuthTypeChange = (value: string | null) => {
    const nextType = (value || 'header') as AdvancedCustomAuthType
    setDraft((current) => ({
      ...current,
      auth:
        nextType === 'none'
          ? { type: 'none' }
          : {
              type: nextType,
              name:
                current.auth?.name ||
                (nextType === 'header' ? 'Authorization' : ''),
              value: current.auth?.value || '',
            },
    }))
  }

  const handleSave = () => {
    if (validationError) return
    props.onSave(normalizeBalanceQueryConfig(draft))
    props.onOpenChange(false)
  }

  const handleTest = async () => {
    if (!props.channelId || validationError) return
    setTestLoading(true)
    setTestResult(null)
    try {
      const response = await testChannelBalanceQuery(
        props.channelId,
        normalizeBalanceQueryConfig(draft)
      )
      if (!response.success) {
        throw new Error(response.message || t('Balance query test failed'))
      }
      if (response.mapping_success === false) {
        setTestResult({
          success: false,
          message: response.message || t('Response mapping did not match'),
          details: response.raw_response,
        })
        return
      }
      setTestResult({
        success: true,
        message: t('Balance query test succeeded'),
        details: response.balance_info
          ? JSON.stringify(response.balance_info, null, 2)
          : undefined,
      })
    } catch (error) {
      setTestResult({
        success: false,
        message:
          error instanceof Error
            ? error.message
            : t('Balance query test failed'),
      })
    } finally {
      setTestLoading(false)
    }
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Upstream Balance Query')}
      description={t(
        'Configure how this channel reads its balance without adding version paths to the Base URL.'
      )}
      contentHeight='85vh'
      bodyClassName='space-y-5 overflow-y-auto'
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            onClick={() => props.onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          {mode === 'custom' && (
            <Button
              type='button'
              variant='outline'
              onClick={handleTest}
              disabled={
                !props.channelId || Boolean(validationError) || testLoading
              }
            >
              {testLoading ? (
                <Loader2 className='h-4 w-4 animate-spin' />
              ) : (
                <TestTube2 className='h-4 w-4' />
              )}
              {testLoading ? t('Testing...') : t('Test query')}
            </Button>
          )}
          <Button
            type='button'
            onClick={handleSave}
            disabled={Boolean(validationError)}
          >
            {t('Save balance query')}
          </Button>
        </>
      }
    >
      <div className='space-y-2'>
        <Label htmlFor='balance-query-mode'>{t('Query mode')}</Label>
        <Select items={modeItems} value={mode} onValueChange={handleModeChange}>
          <SelectTrigger id='balance-query-mode'>
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            <SelectGroup>
              {BALANCE_QUERY_MODE_OPTIONS.map((option) => (
                <SelectItem key={option.value} value={option.value}>
                  {t(option.label)}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
        <p className='text-muted-foreground text-xs'>
          {t(selectedMode?.description || '')}
        </p>
      </div>

      {mode !== 'disabled' && (
        <div className='space-y-4 rounded-lg border p-4'>
          <div className='flex items-center justify-between gap-4'>
            <div>
              <Label htmlFor='balance-auto-refresh'>
                {t('Automatic balance refresh')}
              </Label>
              <p className='text-muted-foreground text-xs'>
                {t(
                  'The master node periodically updates only enabled channels with this option turned on.'
                )}
              </p>
            </div>
            <Switch
              id='balance-auto-refresh'
              checked={draft.auto_refresh === true}
              onCheckedChange={(checked) =>
                setDraft((current) => ({
                  ...current,
                  auto_refresh: checked,
                  refresh_minutes: current.refresh_minutes || 15,
                  low_balance_alert: checked
                    ? current.low_balance_alert
                    : false,
                }))
              }
            />
          </div>

          {draft.auto_refresh && (
            <>
              <div className='space-y-2'>
                <Label htmlFor='balance-refresh-minutes'>
                  {t('Refresh interval in minutes')}
                </Label>
                <Input
                  id='balance-refresh-minutes'
                  type='number'
                  min={1}
                  max={1440}
                  value={draft.refresh_minutes || 15}
                  onChange={(event) =>
                    setDraft((current) => ({
                      ...current,
                      refresh_minutes: Number(event.target.value),
                    }))
                  }
                />
              </div>
              <div className='flex items-center justify-between gap-4'>
                <div>
                  <Label htmlFor='balance-low-alert'>
                    {t('Low balance administrator alert')}
                  </Label>
                  <p className='text-muted-foreground text-xs'>
                    {t(
                      'Notify the root administrator once when the balance crosses below the threshold; it is rearmed after recovery.'
                    )}
                  </p>
                </div>
                <Switch
                  id='balance-low-alert'
                  checked={draft.low_balance_alert === true}
                  onCheckedChange={(checked) =>
                    setDraft((current) => ({
                      ...current,
                      low_balance_alert: checked,
                    }))
                  }
                />
              </div>
              {draft.low_balance_alert && (
                <div className='space-y-2'>
                  <Label htmlFor='balance-low-threshold'>
                    {t('Low balance threshold')}
                  </Label>
                  <Input
                    id='balance-low-threshold'
                    inputMode='decimal'
                    placeholder='10'
                    value={draft.low_balance_threshold || ''}
                    onChange={(event) =>
                      setDraft((current) => ({
                        ...current,
                        low_balance_threshold: event.target.value,
                      }))
                    }
                  />
                  <p className='text-muted-foreground text-xs'>
                    {t(
                      'The threshold uses the final displayed balance unit after applying the multiplier.'
                    )}
                  </p>
                </div>
              )}
            </>
          )}
        </div>
      )}

      {usesAccountToken && (
        <Alert>
          <AlertTitle>
            {t(
              mode === 'new_api' || mode === 'auto'
                ? 'NewAPI account access token required'
                : 'OneAPI account access token required'
            )}
          </AlertTitle>
          <AlertDescription className='space-y-3'>
            <p>
              {t(
                'The /api/user/self endpoint requires an account access token. A normal channel API key only exposes that key quota and cannot read the account wallet balance.'
              )}
            </p>
            {mode === 'auto' && (
              <p>
                {t(
                  'The channel still follows its type automatically; the credentials below are saved only for the NewAPI account balance query.'
                )}
              </p>
            )}
            <div className='space-y-2'>
              <Label htmlFor='account-balance-auth-value'>
                {t('Account access token')}
              </Label>
              <div className='flex items-center gap-2'>
                <Input
                  id='account-balance-auth-value'
                  className='min-w-0 flex-1 font-mono'
                  type={tokenVisible ? 'text' : 'password'}
                  autoComplete='new-password'
                  placeholder={t(
                    draft.auth_configured
                      ? 'Configured; leave blank to keep the current token'
                      : 'Enter the upstream account access token'
                  )}
                  value={displayedToken}
                  onChange={(event) => {
                    setTokenEdited(true)
                    setDraft((current) => ({
                      ...current,
                      auth: {
                        type: 'header',
                        name: 'Authorization',
                        value: event.target.value,
                      },
                    }))
                  }}
                />
                <Button
                  type='button'
                  variant='outline'
                  size='icon'
                  title={t(tokenVisible ? 'Hide token' : 'Show full token')}
                  aria-label={t(
                    tokenVisible ? 'Hide token' : 'Show full token'
                  )}
                  disabled={
                    props.savedTokenLoading ||
                    (!displayedToken &&
                      (!draft.auth_configured ||
                        !props.canRevealSavedToken ||
                        !props.onRevealSavedToken))
                  }
                  onClick={async () => {
                    if (tokenVisible) {
                      setTokenVisible(false)
                      return
                    }
                    setTokenVisible(true)
                    if (!displayedToken && draft.auth_configured) {
                      await props.onRevealSavedToken?.()
                    }
                  }}
                >
                  {tokenVisibilityIcon}
                </Button>
                <Button
                  type='button'
                  variant='outline'
                  size='icon'
                  title={t('Copy token')}
                  aria-label={t('Copy token')}
                  disabled={
                    props.savedTokenLoading ||
                    (!displayedToken &&
                      (!draft.auth_configured ||
                        !props.canRevealSavedToken ||
                        !props.onCopySavedToken))
                  }
                  onClick={async () => {
                    if (displayedToken) {
                      await copyToClipboard(displayedToken)
                      return
                    }
                    await props.onCopySavedToken?.()
                  }}
                >
                  <Copy className='h-4 w-4' />
                </Button>
              </div>
            </div>
            <div className='space-y-2'>
              <Label htmlFor='account-balance-user-id'>
                {t('Upstream account user ID')}
              </Label>
              <Input
                id='account-balance-user-id'
                inputMode='numeric'
                placeholder={t('For example: 300')}
                value={draft.account_user_id || ''}
                onChange={(event) =>
                  setDraft((current) => ({
                    ...current,
                    account_user_id: event.target.value,
                  }))
                }
              />
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Some NewAPI and OneAPI versions require the New-Api-User header. Find this ID on the upstream personal settings page.'
                )}
              </p>
            </div>
          </AlertDescription>
        </Alert>
      )}

      {mode === 'gcp_trial_credit' && (
        <div className='space-y-4'>
          <Alert>
            <AlertTitle>{t('Vertex trial credit activation steps')}</AlertTitle>
            <AlertDescription>
              <ol className='list-decimal space-y-1 pl-5'>
                <li>
                  {t(
                    'Create a US multi-region BigQuery dataset named billing_export.'
                  )}
                </li>
                <li>
                  {t(
                    'Enable Standard usage cost under Billing > Billing export.'
                  )}
                </li>
                <li>
                  {t(
                    'Grant the reused Vertex service account BigQuery Job User and BigQuery Data Viewer.'
                  )}
                </li>
                <li>
                  {t(
                    'Wait for Google to create the standard billing export table; billing data is updated daily.'
                  )}
                </li>
              </ol>
            </AlertDescription>
          </Alert>

          <div className='grid gap-4 rounded-lg border p-4 sm:grid-cols-2'>
            <div className='space-y-2'>
              <Label htmlFor='gcp-billing-account-id'>
                {t('Billing account ID')}
              </Label>
              <Input
                id='gcp-billing-account-id'
                placeholder='0112D2-3D1562-101A70'
                value={draft.gcp_trial?.billing_account_id || ''}
                onChange={(event) =>
                  updateGCPTrial('billing_account_id', event.target.value)
                }
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='gcp-query-project-id'>
                {t('BigQuery project ID')}
              </Label>
              <Input
                id='gcp-query-project-id'
                placeholder='api-505117'
                value={draft.gcp_trial?.query_project_id || ''}
                onChange={(event) =>
                  updateGCPTrial('query_project_id', event.target.value)
                }
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='gcp-dataset-id'>{t('BigQuery dataset ID')}</Label>
              <Input
                id='gcp-dataset-id'
                placeholder='billing_export'
                value={draft.gcp_trial?.dataset_id || ''}
                onChange={(event) =>
                  updateGCPTrial('dataset_id', event.target.value)
                }
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='gcp-credential-channel-id'>
                {t('Vertex credential channel ID')}
              </Label>
              <Input
                id='gcp-credential-channel-id'
                inputMode='numeric'
                placeholder={t('Leave blank to reuse this channel JSON')}
                value={draft.gcp_trial?.credential_channel_id || ''}
                onChange={(event) =>
                  updateGCPTrial(
                    'credential_channel_id',
                    event.target.value ? Number(event.target.value) : 0
                  )
                }
              />
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Use another Vertex channel ID only when that service account has the BigQuery permissions.'
                )}
              </p>
            </div>
            <div className='space-y-2'>
              <Label htmlFor='gcp-total-credit'>
                {t('Trial credit total')}
              </Label>
              <Input
                id='gcp-total-credit'
                value={draft.gcp_trial?.total_amount || '300'}
                readOnly
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='gcp-baseline-used'>
                {t('Historical used baseline')}
              </Label>
              <Input
                id='gcp-baseline-used'
                inputMode='decimal'
                value={draft.gcp_trial?.baseline_used || ''}
                onChange={(event) =>
                  updateGCPTrial('baseline_used', event.target.value)
                }
              />
            </div>
            <div className='space-y-2 sm:col-span-2'>
              <Label htmlFor='gcp-baseline-time'>
                {t('Tracking baseline time')}
              </Label>
              <Input
                id='gcp-baseline-time'
                type='datetime-local'
                value={formatLocalDateTime(draft.gcp_trial?.baseline_at)}
                onChange={(event) =>
                  updateGCPTrial(
                    'baseline_at',
                    event.target.value
                      ? Math.floor(
                          new Date(event.target.value).getTime() / 1000
                        )
                      : 0
                  )
                }
              />
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Only promotional credits with usage after this time are added to the historical baseline, preventing double counting.'
                )}
              </p>
            </div>
          </div>

          <Alert>
            <AlertTitle>{t('Expected billing table')}</AlertTitle>
            <AlertDescription className='font-mono break-all'>
              {gcpTrialTable ||
                t('Complete the billing account, project, and dataset fields.')}
            </AlertDescription>
          </Alert>
        </div>
      )}

      {mode === 'custom' && (
        <>
          <div className='grid gap-4 sm:grid-cols-2'>
            <div className='space-y-2 sm:col-span-2'>
              <Label htmlFor='balance-query-url'>
                {t('Complete request URL')}
              </Label>
              <Input
                id='balance-query-url'
                placeholder='https://api.example.com/v1/user/balance'
                value={draft.url || previewURL}
                onChange={(event) =>
                  setDraft((current) => ({
                    ...current,
                    url: event.target.value,
                    path: '',
                  }))
                }
              />
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Enter the complete balance endpoint manually. It is not joined with the channel Base URL.'
                )}
              </p>
            </div>

            <div className='space-y-2'>
              <Label htmlFor='balance-query-method'>
                {t('Request method')}
              </Label>
              <Select
                items={[
                  { value: 'GET', label: 'GET' },
                  { value: 'POST', label: 'POST' },
                ]}
                value={draft.method || 'GET'}
                onValueChange={(value) =>
                  setDraft((current) => ({
                    ...current,
                    method: value || 'GET',
                    body: value === 'POST' ? current.body || '' : '',
                  }))
                }
              >
                <SelectTrigger id='balance-query-method'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectGroup>
                    <SelectItem value='GET'>GET</SelectItem>
                    <SelectItem value='POST'>POST</SelectItem>
                  </SelectGroup>
                </SelectContent>
              </Select>
            </div>

            {draft.method === 'POST' && (
              <div className='space-y-2 sm:col-span-2'>
                <Label htmlFor='balance-query-body'>
                  {t('JSON request body')}
                </Label>
                <Textarea
                  id='balance-query-body'
                  className='min-h-28 font-mono'
                  placeholder={'{\n  "account_id": "..."\n}'}
                  value={draft.body || ''}
                  onChange={(event) =>
                    setDraft((current) => ({
                      ...current,
                      body: event.target.value,
                    }))
                  }
                />
                <p className='text-muted-foreground text-xs'>
                  {t(
                    'The request body must be valid JSON. Use {api_key} to insert the channel key.'
                  )}
                </p>
              </div>
            )}

            <div className='space-y-2'>
              <Label htmlFor='balance-query-auth-type'>
                {t('Authentication type')}
              </Label>
              <Select
                items={BALANCE_QUERY_AUTH_OPTIONS.map((option) => ({
                  value: option.value,
                  label: t(option.label),
                }))}
                value={authType}
                onValueChange={handleAuthTypeChange}
              >
                <SelectTrigger id='balance-query-auth-type'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectGroup>
                    {BALANCE_QUERY_AUTH_OPTIONS.map((option) => (
                      <SelectItem key={option.value} value={option.value}>
                        {t(option.label)}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </div>

            {authType !== 'none' && (
              <>
                <div className='space-y-2'>
                  <Label htmlFor='balance-query-auth-name'>
                    {authType === 'header'
                      ? t('Header name')
                      : t('Query parameter name')}
                  </Label>
                  <Input
                    id='balance-query-auth-name'
                    placeholder={
                      authType === 'header' ? 'Authorization' : 'api_key'
                    }
                    value={draft.auth?.name || ''}
                    onChange={(event) =>
                      setDraft((current) => ({
                        ...current,
                        auth: {
                          type: authType,
                          name: event.target.value,
                          value: current.auth?.value || '',
                        },
                      }))
                    }
                  />
                </div>
                <div className='space-y-2 sm:col-span-2'>
                  <Label htmlFor='balance-query-auth-value'>
                    {t('Authentication value')}
                  </Label>
                  <div className='flex items-center gap-2'>
                    <Input
                      id='balance-query-auth-value'
                      className='min-w-0 flex-1 font-mono'
                      type={tokenVisible ? 'text' : 'password'}
                      placeholder={
                        draft.auth_configured
                          ? t(
                              'Configured; leave blank to keep the current token'
                            )
                          : 'Bearer {api_key}'
                      }
                      value={displayedToken}
                      onChange={(event) => {
                        setTokenEdited(true)
                        setDraft((current) => ({
                          ...current,
                          auth: {
                            type: authType,
                            name: current.auth?.name || '',
                            value: event.target.value,
                          },
                        }))
                      }}
                    />
                    <Button
                      type='button'
                      variant='outline'
                      size='icon'
                      title={t(tokenVisible ? 'Hide token' : 'Show full token')}
                      aria-label={t(
                        tokenVisible ? 'Hide token' : 'Show full token'
                      )}
                      disabled={
                        props.savedTokenLoading ||
                        (!displayedToken &&
                          (!draft.auth_configured ||
                            !props.canRevealSavedToken ||
                            !props.onRevealSavedToken))
                      }
                      onClick={async () => {
                        if (tokenVisible) {
                          setTokenVisible(false)
                          return
                        }
                        setTokenVisible(true)
                        if (!displayedToken && draft.auth_configured) {
                          await props.onRevealSavedToken?.()
                        }
                      }}
                    >
                      {tokenVisibilityIcon}
                    </Button>
                    <Button
                      type='button'
                      variant='outline'
                      size='icon'
                      title={t('Copy token')}
                      aria-label={t('Copy token')}
                      disabled={
                        props.savedTokenLoading ||
                        (!displayedToken &&
                          (!draft.auth_configured ||
                            !props.canRevealSavedToken ||
                            !props.onCopySavedToken))
                      }
                      onClick={async () => {
                        if (displayedToken) {
                          await copyToClipboard(displayedToken)
                          return
                        }
                        await props.onCopySavedToken?.()
                      }}
                    >
                      <Copy className='h-4 w-4' />
                    </Button>
                  </div>
                  <p className='text-muted-foreground text-xs'>
                    {t('Use {api_key} to insert the channel key.')}
                  </p>
                </div>
              </>
            )}

            <div className='space-y-3 sm:col-span-2'>
              <div className='flex items-center justify-between gap-3'>
                <div>
                  <Label>{t('Additional headers')}</Label>
                  <p className='text-muted-foreground text-xs'>
                    {t('Up to 20 additional request headers are supported.')}
                  </p>
                </div>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  disabled={
                    (draft.headers?.length || 0) >= 20 ||
                    (draft.headers || []).some((header) => !header.name?.trim())
                  }
                  onClick={() =>
                    setDraft((current) => ({
                      ...current,
                      headers: [
                        ...(current.headers || []),
                        { name: '', value: '' },
                      ],
                    }))
                  }
                >
                  <Plus className='h-4 w-4' />
                  {t('Add header')}
                </Button>
              </div>
              {(draft.headers || []).map((header, index) => (
                <div
                  key={
                    header.name || header.masked || header.value || 'new-header'
                  }
                  className='grid gap-2 sm:grid-cols-[minmax(0,0.8fr)_minmax(0,1.4fr)_auto]'
                >
                  <Input
                    aria-label={t('Additional header name')}
                    placeholder='X-Account-ID'
                    value={header.name || ''}
                    onChange={(event) =>
                      setDraft((current) => ({
                        ...current,
                        headers: (current.headers || []).map(
                          (item, itemIndex) =>
                            itemIndex === index
                              ? { ...item, name: event.target.value }
                              : item
                        ),
                      }))
                    }
                  />
                  <Input
                    aria-label={t('Additional header value')}
                    className='font-mono'
                    placeholder={
                      header.configured
                        ? t('Configured; leave blank to keep the current value')
                        : t('Header value or {api_key}')
                    }
                    value={header.value || ''}
                    onChange={(event) =>
                      setDraft((current) => ({
                        ...current,
                        headers: (current.headers || []).map(
                          (item, itemIndex) =>
                            itemIndex === index
                              ? { ...item, value: event.target.value }
                              : item
                        ),
                      }))
                    }
                  />
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon'
                    aria-label={t('Remove header')}
                    onClick={() =>
                      setDraft((current) => ({
                        ...current,
                        headers: (current.headers || []).filter(
                          (_, itemIndex) => itemIndex !== index
                        ),
                      }))
                    }
                  >
                    <Trash2 className='h-4 w-4' />
                  </Button>
                </div>
              ))}
            </div>
          </div>

          <div className='space-y-3 rounded-lg border p-4'>
            <div>
              <h3 className='text-sm font-medium'>{t('Response mapping')}</h3>
              <p className='text-muted-foreground text-xs'>
                {t('Use dotted JSON paths such as data.balance.')}
              </p>
            </div>
            <div className='space-y-2'>
              <Label htmlFor='balance-remaining-mode'>
                {t('Remaining balance calculation')}
              </Label>
              <Select
                items={[
                  {
                    value: 'direct',
                    label: t('Read remaining field directly'),
                  },
                  {
                    value: 'total_minus_used',
                    label: t('Total minus used or frozen'),
                  },
                ]}
                value={draft.remaining_mode || 'direct'}
                onValueChange={(value) =>
                  setDraft((current) => ({
                    ...current,
                    remaining_mode:
                      value === 'total_minus_used'
                        ? 'total_minus_used'
                        : 'direct',
                  }))
                }
              >
                <SelectTrigger id='balance-remaining-mode'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectGroup>
                    <SelectItem value='direct'>
                      {t('Read remaining field directly')}
                    </SelectItem>
                    <SelectItem value='total_minus_used'>
                      {t('Total minus used or frozen')}
                    </SelectItem>
                  </SelectGroup>
                </SelectContent>
              </Select>
            </div>
            <div className='grid gap-4 sm:grid-cols-2'>
              {draft.remaining_mode !== 'total_minus_used' && (
                <div className='space-y-2'>
                  <Label htmlFor='balance-remaining-path'>
                    {t('Remaining balance path')} *
                  </Label>
                  <Input
                    id='balance-remaining-path'
                    placeholder='balance'
                    value={draft.response?.remaining_path || ''}
                    onChange={(event) =>
                      updateResponsePath('remaining_path', event.target.value)
                    }
                  />
                </div>
              )}
              <div className='space-y-2'>
                <Label htmlFor='balance-total-path'>
                  {t('Total path')}
                  {draft.remaining_mode === 'total_minus_used' ? ' *' : ''}
                </Label>
                <Input
                  id='balance-total-path'
                  placeholder='total'
                  value={draft.response?.total_path || ''}
                  onChange={(event) =>
                    updateResponsePath('total_path', event.target.value)
                  }
                />
              </div>
              <div className='space-y-2'>
                <Label htmlFor='balance-used-path'>
                  {t('Used or frozen path')}
                  {draft.remaining_mode === 'total_minus_used' ? ' *' : ''}
                </Label>
                <Input
                  id='balance-used-path'
                  placeholder='used'
                  value={draft.response?.used_path || ''}
                  onChange={(event) =>
                    updateResponsePath('used_path', event.target.value)
                  }
                />
              </div>
              <div className='space-y-2'>
                <Label htmlFor='balance-currency-path'>
                  {t('Currency path')}
                </Label>
                <Input
                  id='balance-currency-path'
                  placeholder='currency'
                  value={draft.response?.currency_path || ''}
                  onChange={(event) =>
                    updateResponsePath('currency_path', event.target.value)
                  }
                />
              </div>
              <div className='space-y-2'>
                <Label htmlFor='balance-active-path'>
                  {t('Active status path')}
                </Label>
                <Input
                  id='balance-active-path'
                  placeholder='is_active'
                  value={draft.response?.active_path || ''}
                  onChange={(event) =>
                    updateResponsePath('active_path', event.target.value)
                  }
                />
              </div>
              <div className='space-y-2'>
                <Label htmlFor='balance-unlimited-path'>
                  {t('Unlimited status path')}
                </Label>
                <Input
                  id='balance-unlimited-path'
                  placeholder='unlimited'
                  value={draft.response?.unlimited_path || ''}
                  onChange={(event) =>
                    updateResponsePath('unlimited_path', event.target.value)
                  }
                />
              </div>
              <div className='space-y-2'>
                <Label htmlFor='balance-success-path'>
                  {t('Success status path')}
                </Label>
                <Input
                  id='balance-success-path'
                  placeholder='success'
                  value={draft.response?.success_path || ''}
                  onChange={(event) =>
                    updateResponsePath('success_path', event.target.value)
                  }
                />
              </div>
              <div className='space-y-2'>
                <Label htmlFor='balance-success-value'>
                  {t('Expected success value')}
                </Label>
                <Input
                  id='balance-success-value'
                  placeholder='true, 0, or success'
                  value={draft.response?.success_value || ''}
                  onChange={(event) =>
                    updateResponsePath('success_value', event.target.value)
                  }
                />
              </div>
            </div>
          </div>

          <div className='grid gap-4 sm:grid-cols-2'>
            <div className='space-y-2'>
              <Label htmlFor='balance-unit'>{t('Balance unit')}</Label>
              <Select
                items={BALANCE_QUERY_UNIT_OPTIONS.map((option) => ({
                  value: option.value,
                  label: t(option.label),
                }))}
                value={draft.unit || 'money'}
                onValueChange={(value) =>
                  setDraft((current) => ({
                    ...current,
                    unit: (value || 'money') as ChannelBalanceUnit,
                  }))
                }
              >
                <SelectTrigger id='balance-unit'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectGroup>
                    {BALANCE_QUERY_UNIT_OPTIONS.map((option) => (
                      <SelectItem key={option.value} value={option.value}>
                        {t(option.label)}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </div>
            <div className='space-y-2'>
              <Label htmlFor='balance-metric-kind'>{t('Metric type')}</Label>
              <Select
                items={BALANCE_QUERY_METRIC_OPTIONS.map((option) => ({
                  value: option.value,
                  label: t(option.label),
                }))}
                value={draft.metric_kind || 'custom'}
                onValueChange={(value) =>
                  setDraft((current) => ({
                    ...current,
                    metric_kind: (value ||
                      'custom') as ChannelBalanceMetricKind,
                  }))
                }
              >
                <SelectTrigger id='balance-metric-kind'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectGroup>
                    {BALANCE_QUERY_METRIC_OPTIONS.map((option) => (
                      <SelectItem key={option.value} value={option.value}>
                        {t(option.label)}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </div>
            <div className='space-y-2'>
              <Label htmlFor='balance-currency'>{t('Default currency')}</Label>
              <Input
                id='balance-currency'
                placeholder='USD'
                value={draft.currency || ''}
                onChange={(event) =>
                  setDraft((current) => ({
                    ...current,
                    currency: event.target.value,
                  }))
                }
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='balance-display-unit'>{t('Display unit')}</Label>
              <Input
                id='balance-display-unit'
                placeholder='$'
                value={draft.display_unit || ''}
                onChange={(event) =>
                  setDraft((current) => ({
                    ...current,
                    display_unit: event.target.value,
                  }))
                }
              />
            </div>
            <div className='space-y-2 sm:col-span-2'>
              <Label htmlFor='balance-multiplier'>{t('Multiplier')}</Label>
              <Input
                id='balance-multiplier'
                inputMode='decimal'
                value={draft.multiplier || '1'}
                onChange={(event) =>
                  setDraft((current) => ({
                    ...current,
                    multiplier: event.target.value,
                  }))
                }
              />
              <p className='text-muted-foreground text-xs'>
                {t(
                  'The parsed remaining, total, and used values are multiplied by this number.'
                )}
              </p>
            </div>
          </div>
        </>
      )}

      {mode === 'custom' && testResult && (
        <Alert variant={testResult.success ? 'default' : 'destructive'}>
          <AlertTitle>
            {testResult.success
              ? t('Balance query test succeeded')
              : t('Balance query test needs attention')}
          </AlertTitle>
          <AlertDescription className='space-y-2'>
            <p>{testResult.message}</p>
            {testResult.details && (
              <pre className='max-h-64 overflow-auto rounded-md bg-black/10 p-3 font-mono text-xs whitespace-pre-wrap dark:bg-white/5'>
                {testResult.details}
              </pre>
            )}
          </AlertDescription>
        </Alert>
      )}

      {previewURL && (
        <Alert>
          <AlertTitle>{t('Final request URL')}</AlertTitle>
          <AlertDescription className='font-mono break-all'>
            {previewURL}
          </AlertDescription>
        </Alert>
      )}

      {mode === 'auto' && !previewURL && (
        <Alert>
          <AlertTitle>{t('Existing channel adapter')}</AlertTitle>
          <AlertDescription>
            {t(
              'This channel type will continue using its existing built-in balance query behavior.'
            )}
          </AlertDescription>
        </Alert>
      )}

      {validationError && (
        <Alert variant='destructive'>
          <AlertTitle>{t('Configuration incomplete')}</AlertTitle>
          <AlertDescription>{t(validationError)}</AlertDescription>
        </Alert>
      )}
    </Dialog>
  )
}

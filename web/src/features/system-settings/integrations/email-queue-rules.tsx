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
import { useEffect, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { getUserFacingErrorMessage } from '@/lib/user-facing-error'

import { updateSystemOption } from '../api'

export type EmailQueueRules = {
  marketing_daily_limit: number
  marketing_per_minute_limit: number
  marketing_user_cooldown_days: number
  marketing_send_start_hour: number
  marketing_send_end_hour: number
  email_max_attempts: number
  email_retry_initial_seconds: number
  email_retry_max_seconds: number
  delivered_retention_days: number
  terminal_retention_days: number
}

const DEFAULT_EMAIL_QUEUE_RULES: EmailQueueRules = {
  marketing_daily_limit: 500,
  marketing_per_minute_limit: 20,
  marketing_user_cooldown_days: 7,
  marketing_send_start_hour: 9,
  marketing_send_end_hour: 20,
  email_max_attempts: 8,
  email_retry_initial_seconds: 30,
  email_retry_max_seconds: 86400,
  delivered_retention_days: 30,
  terminal_retention_days: 90,
}

type EmailQueueRulesCardProps = {
  rules?: EmailQueueRules
  onSaved?: () => void | Promise<void>
}

export function EmailQueueRulesCard(props: EmailQueueRulesCardProps) {
  const { t } = useTranslation()
  const localChanges = useRef(false)
  const [baseline, setBaseline] = useState<EmailQueueRules>(
    props.rules ?? DEFAULT_EMAIL_QUEUE_RULES
  )
  const [draft, setDraft] = useState<EmailQueueRules>(
    props.rules ?? DEFAULT_EMAIL_QUEUE_RULES
  )
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (!props.rules) return
    setBaseline(props.rules)
    if (!localChanges.current) setDraft(props.rules)
  }, [props.rules])

  const error = validateEmailQueueRules(draft, t)
  const dirty = JSON.stringify(draft) !== JSON.stringify(baseline)

  const update = (key: keyof EmailQueueRules, value: number) => {
    setDraft((current) => {
      const next = { ...current, [key]: value }
      localChanges.current = JSON.stringify(next) !== JSON.stringify(baseline)
      return next
    })
  }

  const save = async () => {
    if (error || saving) return
    setSaving(true)
    try {
      const response = await updateSystemOption({
        key: 'EmailDeliveryRules',
        value: JSON.stringify(draft),
      })
      if (!response.success) {
        throw new Error(response.message || 'Failed to save queue rules')
      }
      toast.success(t('Queue rules saved'))
      localChanges.current = false
      setBaseline(draft)
      await props.onSaved?.()
    } catch (saveError) {
      toast.error(
        getUserFacingErrorMessage(saveError, t('Failed to save queue rules'))
      )
    } finally {
      setSaving(false)
    }
  }

  return (
    <Card data-card-hover='false'>
      <CardHeader>
        <CardTitle>{t('Email queue rules')}</CardTitle>
        <CardDescription>
          {t(
            'Marketing quota is reserved before queueing, while delivered usage counts only messages accepted by SMTP.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='space-y-5'>
        <div className='space-y-3'>
          <h3 className='text-sm font-semibold'>{t('Marketing delivery')}</h3>
          <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-5'>
            <RuleNumberField
              label={t('Marketing daily limit')}
              value={draft.marketing_daily_limit}
              min={1}
              max={100000}
              onChange={(value) => update('marketing_daily_limit', value)}
            />
            <RuleNumberField
              label={t('Marketing per-minute limit')}
              value={draft.marketing_per_minute_limit}
              min={1}
              max={1000}
              onChange={(value) => update('marketing_per_minute_limit', value)}
            />
            <RuleNumberField
              label={t('User cooldown (days)')}
              value={draft.marketing_user_cooldown_days}
              min={0}
              max={365}
              onChange={(value) =>
                update('marketing_user_cooldown_days', value)
              }
            />
            <RuleNumberField
              label={t('Send window start (hour)')}
              value={draft.marketing_send_start_hour}
              min={0}
              max={23}
              onChange={(value) => update('marketing_send_start_hour', value)}
            />
            <RuleNumberField
              label={t('Send window end (hour)')}
              value={draft.marketing_send_end_hour}
              min={1}
              max={24}
              onChange={(value) => update('marketing_send_end_hour', value)}
            />
          </div>
        </div>

        <div className='space-y-3'>
          <h3 className='text-sm font-semibold'>
            {t('Retries and retention')}
          </h3>
          <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-5'>
            <RuleNumberField
              label={t('Maximum delivery attempts')}
              value={draft.email_max_attempts}
              min={1}
              max={20}
              onChange={(value) => update('email_max_attempts', value)}
            />
            <RuleNumberField
              label={t('Initial retry delay (seconds)')}
              value={draft.email_retry_initial_seconds}
              min={10}
              max={3600}
              onChange={(value) => update('email_retry_initial_seconds', value)}
            />
            <RuleNumberField
              label={t('Maximum retry delay (seconds)')}
              value={draft.email_retry_max_seconds}
              min={10}
              max={86400}
              onChange={(value) => update('email_retry_max_seconds', value)}
            />
            <RuleNumberField
              label={t('Delivered retention (days)')}
              value={draft.delivered_retention_days}
              min={1}
              max={3650}
              onChange={(value) => update('delivered_retention_days', value)}
            />
            <RuleNumberField
              label={t('Failed or expired retention (days)')}
              value={draft.terminal_retention_days}
              min={1}
              max={3650}
              onChange={(value) => update('terminal_retention_days', value)}
            />
          </div>
        </div>

        <div className='flex flex-wrap items-center justify-between gap-3'>
          <p className='text-destructive text-sm' role='alert'>
            {error}
          </p>
          <div className='ml-auto flex gap-2'>
            <Button
              type='button'
              variant='outline'
              disabled={!props.rules || !dirty || saving}
              onClick={() => {
                localChanges.current = false
                setDraft(baseline)
              }}
            >
              {t('Reset changes')}
            </Button>
            <Button
              type='button'
              disabled={!props.rules || !dirty || Boolean(error) || saving}
              onClick={save}
            >
              {saving ? t('Saving...') : t('Save queue rules')}
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

function RuleNumberField(props: {
  label: string
  value: number
  min: number
  max: number
  onChange: (value: number) => void
}) {
  const id = useId()
  return (
    <div className='space-y-1.5'>
      <Label htmlFor={id}>{props.label}</Label>
      <Input
        id={id}
        type='number'
        value={props.value}
        min={props.min}
        max={props.max}
        aria-invalid={
          !Number.isInteger(props.value) ||
          props.value < props.min ||
          props.value > props.max
        }
        onChange={(event) => props.onChange(Number(event.target.value))}
      />
    </div>
  )
}

function validateEmailQueueRules(
  rules: EmailQueueRules,
  t: (key: string) => string
) {
  if (
    !Number.isInteger(rules.marketing_daily_limit) ||
    rules.marketing_daily_limit < 1 ||
    rules.marketing_daily_limit > 100000
  ) {
    return t('Marketing daily limit must be between 1 and 100000')
  }
  if (
    !Number.isInteger(rules.marketing_per_minute_limit) ||
    rules.marketing_per_minute_limit < 1 ||
    rules.marketing_per_minute_limit > 1000
  ) {
    return t('Marketing per-minute limit must be between 1 and 1000')
  }
  if (rules.marketing_per_minute_limit > rules.marketing_daily_limit) {
    return t('Per-minute limit cannot exceed the daily limit')
  }
  if (
    !Number.isInteger(rules.marketing_user_cooldown_days) ||
    rules.marketing_user_cooldown_days < 0 ||
    rules.marketing_user_cooldown_days > 365
  ) {
    return t('User cooldown must be between 0 and 365 days')
  }
  if (
    !Number.isInteger(rules.marketing_send_start_hour) ||
    !Number.isInteger(rules.marketing_send_end_hour) ||
    rules.marketing_send_start_hour < 0 ||
    rules.marketing_send_start_hour > 23 ||
    rules.marketing_send_end_hour < 1 ||
    rules.marketing_send_end_hour > 24 ||
    rules.marketing_send_start_hour >= rules.marketing_send_end_hour
  ) {
    return t('Send window must be a valid increasing hour range')
  }
  if (
    !Number.isInteger(rules.email_max_attempts) ||
    rules.email_max_attempts < 1 ||
    rules.email_max_attempts > 20
  ) {
    return t('Maximum delivery attempts must be between 1 and 20')
  }
  if (
    !Number.isInteger(rules.email_retry_initial_seconds) ||
    rules.email_retry_initial_seconds < 10 ||
    rules.email_retry_initial_seconds > 3600
  ) {
    return t('Initial retry delay must be between 10 and 3600 seconds')
  }
  if (
    !Number.isInteger(rules.email_retry_max_seconds) ||
    rules.email_retry_max_seconds < rules.email_retry_initial_seconds ||
    rules.email_retry_max_seconds > 86400
  ) {
    return t('Maximum retry delay must not be shorter than the initial delay')
  }
  if (
    !Number.isInteger(rules.delivered_retention_days) ||
    rules.delivered_retention_days < 1 ||
    rules.delivered_retention_days > 3650 ||
    !Number.isInteger(rules.terminal_retention_days) ||
    rules.terminal_retention_days < 1 ||
    rules.terminal_retention_days > 3650
  ) {
    return t('Retention must be between 1 and 3650 days')
  }
  return ''
}

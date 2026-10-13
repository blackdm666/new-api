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
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { StatusBadge, type StatusVariant } from '@/components/status-badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { toIntlLocale } from '@/i18n/languages'
import { formatTimestampRelative, formatTimestampToDate } from '@/lib/format'

import type { CyberGuardOverview } from '../types'

function Row(props: { label: string; children: ReactNode }) {
  return (
    <div className='flex items-baseline justify-between gap-4 py-1.5 text-sm'>
      <dt className='text-muted-foreground'>{props.label}</dt>
      <dd className='text-right font-medium tabular-nums'>{props.children}</dd>
    </div>
  )
}

const POLL_STATUS_VARIANT: Record<string, StatusVariant> = {
  ok: 'success',
  error: 'danger',
  skipped: 'warning',
}

export function LastPollCard(props: { poll: CyberGuardOverview['last_poll'] }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const poll = props.poll
  const statusLabels: Record<string, string> = {
    ok: t('Succeeded'),
    error: t('Failed'),
    skipped: t('Skipped'),
  }
  return (
    <Card data-card-hover='false'>
      <CardHeader>
        <CardTitle className='flex items-center gap-2'>
          {t('Last poll')}
          <StatusBadge
            copyable={false}
            label={
              poll.at > 0
                ? (statusLabels[poll.status] ?? poll.status)
                : t('Never polled')
            }
            variant={POLL_STATUS_VARIANT[poll.status] ?? 'neutral'}
          />
        </CardTitle>
        <CardDescription>
          {t(
            'The master node polls sub2api for refusals on the configured interval. Skipped means CyberGuard is off or the sub2api admin key is not set.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <dl className='divide-y'>
          <Row label={t('Time')}>
            {formatTimestampToDate(poll.at)}
            {poll.at > 0 && (
              <span className='text-muted-foreground ml-2 font-normal'>
                {formatTimestampRelative(poll.at, 'seconds', locale)}
              </span>
            )}
          </Row>
          <Row label={t('Rows fetched')}>{poll.fetched}</Row>
          <Row label={t('New events')}>{poll.new_events}</Row>
          <Row label={t('Polled up to')}>
            {formatTimestampToDate(poll.cursor_end)}
          </Row>
        </dl>
        {poll.error && (
          <p className='text-destructive mt-3 text-sm break-words'>
            {poll.error}
          </p>
        )}
      </CardContent>
    </Card>
  )
}

export function ReconciliationCard(props: {
  reconciliation: CyberGuardOverview['reconciliation_24h']
}) {
  const { t } = useTranslation()
  const recon = props.reconciliation
  return (
    <Card data-card-hover='false'>
      <CardHeader>
        <CardTitle>{t('Reconciliation (last 24 hours)')}</CardTitle>
        <CardDescription>
          {t(
            'Every refusal recorded by sub2api should end up attributed to a user or marked unattributed. Rows awaiting attribution are still waiting for their usage log.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <dl className='divide-y'>
          <Row label={t('Refusals from sub2api')}>{recon.sub2api_rows}</Row>
          <Row label={t('Attributed')}>{recon.attributed}</Row>
          <Row label={t('Unattributed')}>{recon.unattributed}</Row>
          <Row label={t('Awaiting attribution')}>{recon.pending}</Row>
        </dl>
      </CardContent>
    </Card>
  )
}

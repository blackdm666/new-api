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
import type { TFunction } from 'i18next'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import { EmptyState } from '@/components/empty-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { formatTimestampToDate } from '@/lib/format'

import type { NoticeRecord, NoticeUser } from './types'

function recipientResult(
  row: NoticeUser,
  status: NoticeRecord['status'],
  t: TFunction
) {
  if (status === 'draft') return t('Not submitted')
  if (row.skip_reason === 'missing_email') return t('Skipped: no email address')
  if (row.skip_reason === 'blocked_email') {
    return t('Skipped: delivery restriction')
  }
  if (row.skip_reason === 'invalid_email') {
    return t('Skipped: invalid email address')
  }
  if (row.skip_reason === 'duplicate_email') {
    return t('Skipped: duplicate email address')
  }
  if (status === 'simulated') return t('Simulated only; not delivered')
  switch (row.state) {
    case 'accepted_untracked':
      return t('SMTP accepted; inbox delivery unconfirmed')
    case 'delivered':
      return t('Delivered')
    case 'sending':
      return t('Sending')
    case 'retrying':
      return t('Retrying')
    case 'failed':
      return t('Failed')
    case 'expired':
      return t('Expired')
    case 'queued':
      return t('Queued')
    default:
      return t('Delivery record unavailable')
  }
}

function smtpChannelLabel(channel: string | undefined, t: TFunction) {
  if (channel === 'security') return t('Verification and security SMTP')
  if (channel === 'primary') return t('Primary SMTP fallback')
  if (channel === 'backup') return t('Backup SMTP fallback')
  return t('Not sent yet')
}

export function NoticeRecords(props: {
  records: NoticeRecord[]
  onEdit: (record: NoticeRecord) => void
  preview: boolean
}) {
  const { t } = useTranslation()
  const [detailID, setDetailID] = useState<number | null>(null)
  const detail = props.records.find((record) => record.id === detailID)
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Notification history')}</CardTitle>
        <CardDescription>
          {props.preview
            ? t(
                'Local demo records only. A simulated submission does not mean an email was sent or delivered.'
              )
            : t(
                'Latest 100 notifications. Recipient results refresh automatically. Queueing or SMTP acceptance does not prove inbox delivery.'
              )}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <StaticDataTable
          data={props.records}
          getRowKey={(row) => row.id}
          emptyContent={
            <EmptyState
              className='min-h-32'
              title={t('No notification records yet')}
              description={t(
                'Save a draft or submit a notification to see its record here.'
              )}
            />
          }
          columns={[
            {
              id: 'subject',
              header: t('Subject'),
              cell: (row) => <span className='font-medium'>{row.subject}</span>,
            },
            {
              id: 'recipients',
              header: t('Recipients'),
              cell: (row) =>
                t('{{count}} users', { count: row.recipients.length }),
            },
            {
              id: 'status',
              header: t('Status'),
              cell: (row) => {
                let label = t('Submitted to queue')
                if (row.status === 'draft') label = t('Draft')
                if (row.status === 'simulated') {
                  label = t('Simulated submission')
                }
                return (
                  <Badge
                    variant={row.status === 'draft' ? 'secondary' : 'outline'}
                  >
                    {label}
                  </Badge>
                )
              },
            },
            {
              id: 'operator',
              header: t('Operator'),
              cell: (row) => row.operator,
            },
            {
              id: 'time',
              header: t('Created time'),
              cell: (row) => formatTimestampToDate(row.created_at),
            },
            {
              id: 'actions',
              header: t('Actions'),
              cell: (row) => (
                <div className='flex gap-1'>
                  <Button
                    variant='ghost'
                    size='sm'
                    onClick={() => setDetailID(row.id)}
                  >
                    {t('Details')}
                  </Button>
                  {row.status === 'draft' && (
                    <Button
                      variant='ghost'
                      size='sm'
                      onClick={() => props.onEdit(row)}
                    >
                      {t('Continue editing')}
                    </Button>
                  )}
                </div>
              ),
            },
          ]}
        />
      </CardContent>
      <Dialog
        open={Boolean(detail)}
        onOpenChange={(open) => {
          if (!open) setDetailID(null)
        }}
        title={t('Notification details')}
        description={
          props.preview
            ? t('Demo audit trail; no real email was sent.')
            : t('Per-recipient queue status and actual SMTP channel.')
        }
      >
        {detail && (
          <div className='space-y-4'>
            <h3 className='font-semibold'>{detail.subject}</h3>
            <p className='text-sm leading-6 break-words whitespace-pre-wrap'>
              {detail.body}
            </p>
            <StaticDataTable
              data={detail.recipients}
              getRowKey={(row) => row.id}
              columns={[
                {
                  id: 'name',
                  header: t('User'),
                  cell: (row) => `${row.display_name} · UID ${row.id}`,
                },
                {
                  id: 'email',
                  header: t('Email'),
                  cell: (row) => row.email_masked || t('No email address'),
                },
                {
                  id: 'result',
                  header: t('Result'),
                  cell: (row) => recipientResult(row, detail.status, t),
                },
                {
                  id: 'smtp',
                  header: t('Actual SMTP channel'),
                  cell: (row) => smtpChannelLabel(row.smtp_channel, t),
                },
                {
                  id: 'error',
                  header: t('Failure reason'),
                  cell: (row) => row.last_error || '—',
                },
              ]}
            />
          </div>
        )}
      </Dialog>
    </Card>
  )
}

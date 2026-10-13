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
import { useTranslation } from 'react-i18next'

import { StaticDataTable, TruncatedCell } from '@/components/data-table'
import { EmptyState } from '@/components/empty-state'
import { GroupBadge } from '@/components/group-badge'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { formatTimestampToDate } from '@/lib/format'

import {
  BAN_STATUS_VARIANT,
  banDurationLabel,
  banExpiryLabel,
  banSourceLabel,
  banStatusLabel,
  emailStateLabel,
  endReasonLabel,
} from '../lib/labels'
import type { CyberGuardBanRow } from '../types'
import type { CaseAction } from './case-action-dialog'

function caseAction(row: CyberGuardBanRow, kind: CaseAction['kind']) {
  return { kind, banId: row.id, caseId: row.case_id, username: row.username }
}

export function BansTable(props: {
  rows: CyberGuardBanRow[]
  onAction: (action: CaseAction) => void
}) {
  const { t } = useTranslation()
  return (
    <StaticDataTable
      data={props.rows}
      getRowKey={(row) => row.id}
      emptyContent={
        <EmptyState className='min-h-32' title={t('No ban records yet')} />
      }
      columns={[
        {
          id: 'case',
          header: t('Case'),
          cell: (row) => (
            <div className='space-y-1'>
              <div className='font-mono text-sm'>{row.case_id}</div>
              <div className='text-muted-foreground text-xs'>
                {banSourceLabel(row.source, t)}
              </div>
            </div>
          ),
        },
        {
          id: 'user',
          header: t('User'),
          cell: (row) => (
            <div className='space-y-1'>
              <div className='font-medium'>{row.username || '—'}</div>
              <div className='text-muted-foreground text-xs'>
                UID {row.user_id}
              </div>
            </div>
          ),
        },
        {
          id: 'groups',
          header: t('Groups'),
          cell: (row) => (
            <div className='space-y-1 text-xs'>
              {row.restore_group &&
                row.restore_group !== row.original_group && (
                  <div className='flex items-center gap-1'>
                    <span className='text-muted-foreground'>
                      {t('Before ban')}
                    </span>
                    <GroupBadge group={row.original_group} />
                  </div>
                )}
              <div className='flex items-center gap-1'>
                <span className='text-muted-foreground'>{t('Restore to')}</span>
                <GroupBadge group={row.restore_group || row.original_group} />
              </div>
              <div className='flex items-center gap-1'>
                <span className='text-muted-foreground'>{t('Now')}</span>
                <GroupBadge group={row.current_group} />
              </div>
            </div>
          ),
        },
        {
          id: 'status',
          header: t('Status'),
          cell: (row) => (
            <div className='space-y-1'>
              <StatusBadge
                copyable={false}
                label={banStatusLabel(row.status, t)}
                variant={BAN_STATUS_VARIANT[row.status]}
              />
              {row.end_reason && (
                <div
                  className='text-muted-foreground text-xs'
                  title={row.end_note || undefined}
                >
                  {endReasonLabel(row.end_reason, t)}
                </div>
              )}
            </div>
          ),
        },
        {
          id: 'time',
          header: t('Ban period'),
          cell: (row) => (
            <div className='space-y-1 text-xs tabular-nums'>
              <div>{formatTimestampToDate(row.started_at)}</div>
              {row.ended_at > 0 && (
                <div className='text-muted-foreground'>
                  {t('Ended {{time}}', {
                    time: formatTimestampToDate(row.ended_at),
                  })}
                </div>
              )}
              {row.status === 'active' && (
                <div className='text-muted-foreground'>
                  {t('Restores: {{time}}', {
                    time: banExpiryLabel(row.expires_at, t),
                  })}
                </div>
              )}
              <div className='text-muted-foreground'>
                {banDurationLabel(row.ban_hours, t)}
              </div>
            </div>
          ),
        },
        {
          id: 'reason',
          header: t('Reason'),
          cell: (row) => (
            <div className='space-y-1'>
              <TruncatedCell className='max-w-56'>
                {row.reason || '—'}
              </TruncatedCell>
              {row.units_summary && (
                <div className='text-muted-foreground text-xs'>
                  {row.units_summary}
                </div>
              )}
            </div>
          ),
        },
        {
          id: 'email',
          header: t('Ban email'),
          cell: (row) => (
            <div className='space-y-1 text-xs'>
              <div title={row.email_error || undefined}>
                {emailStateLabel(row.email_state, row.email_delivery_state, t)}
              </div>
              {row.status === 'active' && (
                <Button
                  type='button'
                  size='xs'
                  variant='ghost'
                  onClick={() => props.onAction(caseAction(row, 'resend'))}
                >
                  {t('Resend')}
                </Button>
              )}
            </div>
          ),
        },
        {
          id: 'actions',
          header: t('Actions'),
          cell: (row) => (
            <div className='flex flex-wrap gap-1'>
              {row.status === 'active' && (
                <Button
                  type='button'
                  size='sm'
                  variant='outline'
                  onClick={() => props.onAction(caseAction(row, 'unban'))}
                >
                  {t('Unban')}
                </Button>
              )}
              {row.status === 'observed' && (
                <>
                  <Button
                    type='button'
                    size='sm'
                    variant='destructive'
                    onClick={() => props.onAction(caseAction(row, 'promote'))}
                  >
                    {t('Apply ban')}
                  </Button>
                  <Button
                    type='button'
                    size='sm'
                    variant='ghost'
                    onClick={() => props.onAction(caseAction(row, 'dismiss'))}
                  >
                    {t('Dismiss')}
                  </Button>
                </>
              )}
            </div>
          ),
        },
      ]}
    />
  )
}

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
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { StaticDataTable, TruncatedCell } from '@/components/data-table'
import { EmptyState } from '@/components/empty-state'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { formatTimestampToDate } from '@/lib/format'

import {
  CATEGORY_VARIANT,
  EVENT_STATE_VARIANT,
  categoryLabel,
  eventStateLabel,
} from '../lib/labels'
import { usageLogSearchForEvent } from '../lib/search'
import type { CyberGuardEvent } from '../types'

export function EventsTable(props: {
  rows: CyberGuardEvent[]
  onVoid: (event: CyberGuardEvent) => void
}) {
  const { t } = useTranslation()
  return (
    <StaticDataTable
      data={props.rows}
      getRowKey={(row) => row.id}
      emptyContent={
        <EmptyState
          className='min-h-32'
          title={t('No matching events')}
          description={t(
            'Only refusals polled after CyberGuard was turned on are recorded.'
          )}
        />
      }
      columns={[
        {
          id: 'time',
          header: t('Time'),
          cell: (row) => (
            <span className='text-xs tabular-nums'>
              {formatTimestampToDate(row.occurred_at)}
            </span>
          ),
        },
        {
          id: 'user',
          header: t('User'),
          cell: (row) =>
            row.user_id > 0 ? (
              <div className='space-y-1'>
                <div className='font-medium'>{row.username}</div>
                <div className='text-muted-foreground text-xs'>
                  UID {row.user_id}
                  {row.token_name ? ` · ${row.token_name}` : ''}
                </div>
              </div>
            ) : (
              <span className='text-muted-foreground text-xs'>
                {t('Not attributed')}
              </span>
            ),
        },
        {
          id: 'category',
          header: t('Category'),
          cell: (row) => (
            <StatusBadge
              copyable={false}
              label={categoryLabel(row.category, t)}
              variant={CATEGORY_VARIANT[row.category]}
            />
          ),
        },
        {
          id: 'state',
          header: t('Result'),
          cell: (row) => (
            <div className='space-y-1' title={row.void_reason || undefined}>
              <StatusBadge
                copyable={false}
                label={eventStateLabel(row.state, t)}
                variant={EVENT_STATE_VARIANT[row.state]}
              />
              {row.state === 'counted' && row.is_new_unit && (
                <div className='text-muted-foreground text-xs'>
                  {t('New session')}
                </div>
              )}
            </div>
          ),
        },
        {
          id: 'model',
          header: t('Model'),
          cell: (row) => (
            <div className='space-y-1'>
              <div className='text-sm'>{row.model || '—'}</div>
              <div className='text-muted-foreground text-xs'>
                {[row.endpoint, row.status_code || '']
                  .filter(Boolean)
                  .join(' · ')}
              </div>
            </div>
          ),
        },
        {
          id: 'account',
          header: t('sub2api account'),
          cell: (row) => {
            if (row.account_id <= 0) return '—'
            return row.account_name || `#${row.account_id}`
          },
        },
        {
          id: 'request',
          header: t('Request ID'),
          cell: (row) =>
            row.sub2api_request_id ? (
              <Link
                to='/usage-logs/$section'
                params={{ section: 'common' }}
                search={usageLogSearchForEvent(
                  row.sub2api_request_id,
                  row.occurred_at
                )}
                className='text-primary block max-w-40 truncate font-mono text-xs hover:underline'
                title={t('Open the usage log of this request')}
              >
                {row.sub2api_request_id}
              </Link>
            ) : (
              '—'
            ),
        },
        {
          id: 'message',
          header: t('Upstream message'),
          cell: (row) => (
            <TruncatedCell className='max-w-64 text-xs'>
              {row.message || '—'}
            </TruncatedCell>
          ),
        },
        {
          id: 'actions',
          header: t('Actions'),
          cell: (row) =>
            row.state === 'void' ? null : (
              <Button
                type='button'
                size='sm'
                variant='ghost'
                onClick={() => props.onVoid(row)}
              >
                {t('Void')}
              </Button>
            ),
        },
      ]}
    />
  )
}

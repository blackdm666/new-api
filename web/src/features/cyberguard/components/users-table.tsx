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

import { StaticDataTable } from '@/components/data-table'
import { EmptyState } from '@/components/empty-state'
import { GroupBadge } from '@/components/group-badge'
import { StatusBadge } from '@/components/status-badge'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { formatTimestampToDate } from '@/lib/format'
import { ROLE } from '@/lib/roles'

import {
  BAN_STATUS_VARIANT,
  banExpiryLabel,
  banStatusLabel,
} from '../lib/labels'
import type { CyberGuardUserRow } from '../types'
import type { CaseAction } from './case-action-dialog'

export function UsersTable(props: {
  rows: CyberGuardUserRow[]
  onBan: (user: CyberGuardUserRow) => void
  onAction: (action: CaseAction) => void
  onOpenEvents: (userId: number) => void
}) {
  const { t } = useTranslation()
  return (
    <StaticDataTable
      data={props.rows}
      getRowKey={(row) => row.user_id}
      emptyContent={
        <EmptyState
          className='min-h-32'
          title={t('No matching users')}
          description={t(
            'Users appear here after their first attributed hit. Search by ID, username or email to find anyone else.'
          )}
        />
      }
      columns={[
        {
          id: 'user',
          header: t('User'),
          cell: (row) => (
            <div className='space-y-1'>
              <div className='flex flex-wrap items-center gap-1 font-medium'>
                {row.username || '—'}
                {row.is_reseller && (
                  <Badge variant='outline'>{t('Reseller')}</Badge>
                )}
                {row.role >= ROLE.ADMIN && (
                  <Badge variant='secondary'>{t('Administrator')}</Badge>
                )}
                {row.is_exempt === true && (
                  <Badge variant='secondary'>{t('Exempt')}</Badge>
                )}
              </div>
              <div className='text-muted-foreground text-xs'>
                UID {row.user_id}
                {row.email ? ` · ${row.email}` : ''}
              </div>
            </div>
          ),
        },
        {
          id: 'group',
          header: t('Current group'),
          cell: (row) => <GroupBadge group={row.current_group} />,
        },
        {
          id: 'units',
          header: t('Flagged sessions (7 / 30 days)'),
          cell: (row) => (
            <span className='tabular-nums'>
              {row.units_7d} / {row.units_30d}
            </span>
          ),
        },
        {
          id: 'hits',
          header: t('Hits (30 days)'),
          cell: (row) => <span className='tabular-nums'>{row.hits_30d}</span>,
        },
        {
          id: 'last',
          header: t('Last hit'),
          cell: (row) => (
            <span className='text-xs tabular-nums'>
              {formatTimestampToDate(row.last_hit_at)}
            </span>
          ),
        },
        {
          id: 'status',
          header: t('Status'),
          cell: (row) =>
            row.ban ? (
              <div className='space-y-1'>
                <StatusBadge
                  copyable={false}
                  label={`${banStatusLabel(row.ban.status, t)} · ${row.ban.case_id}`}
                  variant={BAN_STATUS_VARIANT[row.ban.status]}
                />
                {row.ban.status === 'active' && (
                  <div className='text-muted-foreground text-xs'>
                    {t('Restores: {{time}}', {
                      time: banExpiryLabel(row.ban.expires_at, t),
                    })}
                  </div>
                )}
              </div>
            ) : (
              <span className='text-muted-foreground text-xs'>
                {t('No open record')}
              </span>
            ),
        },
        {
          id: 'actions',
          header: t('Actions'),
          cell: (row) => (
            <UserActions
              row={row}
              onBan={props.onBan}
              onAction={props.onAction}
              onOpenEvents={props.onOpenEvents}
            />
          ),
        },
      ]}
    />
  )
}

function UserActions(props: {
  row: CyberGuardUserRow
  onBan: (user: CyberGuardUserRow) => void
  onAction: (action: CaseAction) => void
  onOpenEvents: (userId: number) => void
}) {
  const { t } = useTranslation()
  const row = props.row
  const ban = row.ban
  // The server refuses these bans too; the title explains the disabled button.
  let banBlockedReason: string | undefined
  if (row.role >= ROLE.ADMIN) {
    banBlockedReason = t('Administrators cannot be banned')
  } else if (row.is_exempt === true) {
    banBlockedReason = t('Exempt users cannot be banned')
  }
  return (
    <div className='flex flex-wrap gap-1'>
      <Button
        type='button'
        size='sm'
        variant='ghost'
        onClick={() => props.onOpenEvents(row.user_id)}
      >
        {t('Events')}
      </Button>
      {ban?.status === 'active' && (
        <Button
          type='button'
          size='sm'
          variant='outline'
          onClick={() =>
            props.onAction({
              kind: 'unban',
              banId: ban.id,
              caseId: ban.case_id,
              username: row.username,
            })
          }
        >
          {t('Unban')}
        </Button>
      )}
      {ban?.status === 'observed' && (
        <Button
          type='button'
          size='sm'
          variant='destructive'
          disabled={banBlockedReason !== undefined}
          title={banBlockedReason}
          onClick={() =>
            props.onAction({
              kind: 'promote',
              banId: ban.id,
              caseId: ban.case_id,
              username: row.username,
            })
          }
        >
          {t('Apply ban')}
        </Button>
      )}
      {!ban && (
        <Button
          type='button'
          size='sm'
          variant='destructive'
          disabled={banBlockedReason !== undefined}
          title={banBlockedReason}
          onClick={() => props.onBan(row)}
        >
          {t('Ban')}
        </Button>
      )}
    </div>
  )
}

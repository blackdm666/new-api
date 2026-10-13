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
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable, TruncatedCell } from '@/components/data-table'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { StatusBadge, type StatusVariant } from '@/components/status-badge'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Skeleton } from '@/components/ui/skeleton'
import { formatTimestampToDate } from '@/lib/format'

import { CYBERGUARD_QUERY_KEY, fetchCyberGuardAccounts } from '../api'
import { schedulableLabel } from '../lib/labels'

const ACCOUNT_STATUS_VARIANT: Record<string, StatusVariant> = {
  active: 'success',
  error: 'danger',
  disabled: 'neutral',
  unknown: 'neutral',
}

/** Hits per sub2api (OpenAI) account, with the account status from sub2api. */
export function AccountsTab() {
  const { t } = useTranslation()
  const [days, setDays] = useState<7 | 30>(7)
  const query = useQuery({
    queryKey: [...CYBERGUARD_QUERY_KEY, 'accounts', days],
    queryFn: () => fetchCyberGuardAccounts(days),
  })

  return (
    <div className='space-y-3'>
      <div className='flex flex-wrap items-center gap-3'>
        <NativeSelect
          aria-label={t('Time range')}
          value={String(days)}
          onChange={(event) => setDays(event.target.value === '30' ? 30 : 7)}
        >
          <NativeSelectOption value='7'>{t('Last 7 days')}</NativeSelectOption>
          <NativeSelectOption value='30'>
            {t('Last 30 days')}
          </NativeSelectOption>
        </NativeSelect>
        <p className='text-muted-foreground text-xs'>
          {t(
            'Account status comes from sub2api and is cached for 60 seconds. Unknown means sub2api is not configured or could not be reached.'
          )}
        </p>
      </div>
      {query.isLoading && <Skeleton className='h-32 w-full' />}
      {query.isError && (
        <ErrorState
          className='min-h-32'
          title={t('Could not load account statistics')}
          description={query.error.message}
          onRetry={() => void query.refetch()}
        />
      )}
      {query.data && (
        <StaticDataTable
          data={query.data}
          getRowKey={(row) => row.account_id}
          emptyContent={
            <EmptyState
              className='min-h-32'
              title={t('No hits in this period')}
            />
          }
          columns={[
            {
              id: 'account',
              header: t('sub2api account'),
              cell: (row) => (
                <div className='space-y-1'>
                  <div className='font-medium'>{row.account_name || '—'}</div>
                  <div className='text-muted-foreground text-xs'>
                    #{row.account_id}
                  </div>
                </div>
              ),
            },
            {
              id: 'cyber',
              header: t('Cybersecurity'),
              cell: (row) => (
                <span className='tabular-nums'>{row.hits_cyber}</span>
              ),
            },
            {
              id: 'bio',
              header: t('Biological risk'),
              cell: (row) => (
                <span className='tabular-nums'>{row.hits_bio}</span>
              ),
            },
            {
              id: 'usage',
              header: t('Usage policy'),
              cell: (row) => (
                <span className='tabular-nums'>{row.hits_usage}</span>
              ),
            },
            {
              id: 'users',
              header: t('Users'),
              cell: (row) => <span className='tabular-nums'>{row.users}</span>,
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
              header: t('Account status'),
              cell: (row) => (
                <StatusBadge
                  copyable={false}
                  label={row.status === 'unknown' ? t('Unknown') : row.status}
                  variant={ACCOUNT_STATUS_VARIANT[row.status] ?? 'warning'}
                />
              ),
            },
            {
              id: 'schedulable',
              header: t('Schedulable'),
              cell: (row) => schedulableLabel(row.schedulable, t),
            },
            {
              id: 'error',
              header: t('Account error'),
              cell: (row) => (
                <TruncatedCell className='max-w-64 text-xs'>
                  {row.error_message || '—'}
                </TruncatedCell>
              ),
            },
          ]}
        />
      )}
    </div>
  )
}

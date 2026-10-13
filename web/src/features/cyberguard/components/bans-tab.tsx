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
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { Import } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Skeleton } from '@/components/ui/skeleton'

import {
  CYBERGUARD_QUERY_KEY,
  fetchCyberGuardBans,
  fetchCyberGuardSettings,
} from '../api'
import { AdoptDialog } from './adopt-dialog'
import { BansTable } from './bans-table'
import { CaseActionDialog, type CaseAction } from './case-action-dialog'
import { ServerPagination } from './server-pagination'
import { UserFilterChip } from './user-filter-chip'

const BAN_STATUSES = ['', 'active', 'observed', 'ended', 'dismissed'] as const

export function BansTab(props: { userId?: number; onClearUser: () => void }) {
  const { t } = useTranslation()
  const [status, setStatus] = useState<(typeof BAN_STATUSES)[number]>('')
  const [pagination, setPagination] = useState({ pageIndex: 0, pageSize: 10 })
  const [action, setAction] = useState<CaseAction | null>(null)
  const [adoptOpen, setAdoptOpen] = useState(false)
  const query = useQuery({
    queryKey: [
      ...CYBERGUARD_QUERY_KEY,
      'bans',
      status,
      props.userId,
      pagination,
    ],
    queryFn: () =>
      fetchCyberGuardBans({
        status: status || undefined,
        user_id: props.userId,
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
      }),
    placeholderData: keepPreviousData,
  })
  const settingsQuery = useQuery({
    queryKey: [...CYBERGUARD_QUERY_KEY, 'settings'],
    queryFn: fetchCyberGuardSettings,
  })
  const statusLabels: Record<(typeof BAN_STATUSES)[number], string> = {
    '': t('All records'),
    active: t('Banned'),
    observed: t('Proposed'),
    ended: t('Ended'),
    dismissed: t('Dismissed'),
  }

  return (
    <div className='space-y-3'>
      <div className='flex flex-wrap items-center gap-2'>
        <NativeSelect
          aria-label={t('Status')}
          value={status}
          onChange={(event) => {
            setStatus(event.target.value as (typeof BAN_STATUSES)[number])
            setPagination((previous) => ({ ...previous, pageIndex: 0 }))
          }}
        >
          {BAN_STATUSES.map((value) => (
            <NativeSelectOption key={value || 'all'} value={value}>
              {statusLabels[value]}
            </NativeSelectOption>
          ))}
        </NativeSelect>
        {props.userId !== undefined && (
          <UserFilterChip userId={props.userId} onClear={props.onClearUser} />
        )}
        <Button
          type='button'
          variant='outline'
          className='ml-auto'
          disabled={!settingsQuery.data}
          onClick={() => setAdoptOpen(true)}
        >
          <Import className='size-4' aria-hidden='true' />
          {t('Import existing bans')}
        </Button>
      </div>
      <div aria-busy={query.isFetching}>
        {query.isLoading && <Skeleton className='h-32 w-full' />}
        {query.isError && (
          <ErrorState
            className='min-h-32'
            title={t('Could not load ban records')}
            description={query.error.message}
            onRetry={() => void query.refetch()}
          />
        )}
        {query.data && (
          <div className='space-y-3'>
            <BansTable rows={query.data.items ?? []} onAction={setAction} />
            <ServerPagination
              total={query.data.total}
              pagination={pagination}
              onPaginationChange={setPagination}
              settled={query.isSuccess && !query.isFetching}
            />
          </div>
        )}
      </div>
      <CaseActionDialog action={action} onClose={() => setAction(null)} />
      {settingsQuery.data && (
        <AdoptDialog
          open={adoptOpen}
          banGroup={settingsQuery.data.ban_group}
          onOpenChange={setAdoptOpen}
        />
      )}
    </div>
  )
}

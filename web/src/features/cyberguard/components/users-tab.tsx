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
import { Search } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Skeleton } from '@/components/ui/skeleton'

import { CYBERGUARD_QUERY_KEY, fetchCyberGuardUsers } from '../api'
import type { CyberGuardUserRow, CyberGuardUserStatus } from '../types'
import { BanUserDialog } from './ban-user-dialog'
import { CaseActionDialog, type CaseAction } from './case-action-dialog'
import { ServerPagination } from './server-pagination'
import { UsersTable } from './users-table'

export function UsersTab(props: { onOpenEvents: (userId: number) => void }) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState('')
  const [keyword, setKeyword] = useState('')
  const [status, setStatus] = useState<CyberGuardUserStatus>('all')
  const [pagination, setPagination] = useState({ pageIndex: 0, pageSize: 10 })
  const [banTarget, setBanTarget] = useState<CyberGuardUserRow | null>(null)
  const [action, setAction] = useState<CaseAction | null>(null)
  const query = useQuery({
    queryKey: [...CYBERGUARD_QUERY_KEY, 'users', keyword, status, pagination],
    queryFn: () =>
      fetchCyberGuardUsers({
        keyword,
        status,
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
      }),
    placeholderData: keepPreviousData,
  })
  const statusLabels: Record<CyberGuardUserStatus, string> = {
    all: t('All users'),
    active: t('Banned'),
    observed: t('Proposed'),
    clean: t('No open record'),
  }

  return (
    <div className='space-y-3'>
      <form
        role='search'
        className='flex flex-wrap items-center gap-2'
        onSubmit={(event) => {
          event.preventDefault()
          setKeyword(draft.trim())
          setPagination((previous) => ({ ...previous, pageIndex: 0 }))
        }}
      >
        <Input
          aria-label={t('Search users')}
          className='w-full sm:w-72'
          value={draft}
          placeholder={t('User ID, username or email')}
          onChange={(event) => setDraft(event.target.value)}
        />
        <NativeSelect
          aria-label={t('Status')}
          value={status}
          onChange={(event) => {
            setStatus(event.target.value as CyberGuardUserStatus)
            setPagination((previous) => ({ ...previous, pageIndex: 0 }))
          }}
        >
          {(Object.keys(statusLabels) as CyberGuardUserStatus[]).map(
            (value) => (
              <NativeSelectOption key={value} value={value}>
                {statusLabels[value]}
              </NativeSelectOption>
            )
          )}
        </NativeSelect>
        <Button type='submit' variant='outline'>
          <Search className='size-4' aria-hidden='true' />
          {t('Search')}
        </Button>
      </form>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Without a search, only users with CyberGuard activity are listed. Search by ID, username or email to ban anyone by hand.'
        )}
      </p>
      <div aria-busy={query.isFetching}>
        {query.isLoading && <Skeleton className='h-32 w-full' />}
        {query.isError && (
          <ErrorState
            className='min-h-32'
            title={t('Could not load users')}
            description={query.error.message}
            onRetry={() => void query.refetch()}
          />
        )}
        {query.data && (
          <div className='space-y-3'>
            <UsersTable
              rows={query.data.items ?? []}
              onBan={setBanTarget}
              onAction={setAction}
              onOpenEvents={props.onOpenEvents}
            />
            <ServerPagination
              total={query.data.total}
              pagination={pagination}
              onPaginationChange={setPagination}
              settled={query.isSuccess && !query.isFetching}
            />
          </div>
        )}
      </div>
      <BanUserDialog user={banTarget} onClose={() => setBanTarget(null)} />
      <CaseActionDialog action={action} onClose={() => setAction(null)} />
    </div>
  )
}

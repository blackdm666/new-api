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

import { CYBERGUARD_QUERY_KEY, fetchCyberGuardEvents } from '../api'
import { categoryLabel, eventStateLabel } from '../lib/labels'
import type { CyberGuardEvent } from '../types'
import { EventsTable } from './events-table'
import { ServerPagination } from './server-pagination'
import { UserFilterChip } from './user-filter-chip'
import { VoidEventDialog } from './void-event-dialog'

const CATEGORIES = ['cyber', 'bio', 'usage', 'session_blocked'] as const
const STATES = [
  'counted',
  'pending',
  'record_only',
  'unattributed',
  'exempt',
  'void',
] as const

export function EventsTab(props: { userId?: number; onClearUser: () => void }) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState('')
  const [keyword, setKeyword] = useState('')
  const [category, setCategory] = useState('')
  const [state, setState] = useState('')
  const [days, setDays] = useState('')
  const [start, setStart] = useState<number | undefined>()
  const [pagination, setPagination] = useState({ pageIndex: 0, pageSize: 20 })
  const [voidTarget, setVoidTarget] = useState<CyberGuardEvent | null>(null)
  const query = useQuery({
    queryKey: [
      ...CYBERGUARD_QUERY_KEY,
      'events',
      { keyword, category, state, start, userId: props.userId },
      pagination,
    ],
    queryFn: () =>
      fetchCyberGuardEvents({
        user_id: props.userId,
        keyword: keyword || undefined,
        category: category || undefined,
        state: state || undefined,
        start,
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
      }),
    placeholderData: keepPreviousData,
  })
  const firstPage = () =>
    setPagination((previous) => ({ ...previous, pageIndex: 0 }))

  return (
    <div className='space-y-3'>
      <form
        role='search'
        className='flex flex-wrap items-center gap-2'
        onSubmit={(event) => {
          event.preventDefault()
          setKeyword(draft.trim())
          firstPage()
        }}
      >
        <Input
          aria-label={t('Search events')}
          className='w-full sm:w-72'
          value={draft}
          placeholder={t('Username, token, request ID or CG-<n>')}
          onChange={(event) => setDraft(event.target.value)}
        />
        <Button type='submit' variant='outline'>
          <Search className='size-4' aria-hidden='true' />
          {t('Search')}
        </Button>
        <NativeSelect
          aria-label={t('Category')}
          value={category}
          onChange={(event) => {
            setCategory(event.target.value)
            firstPage()
          }}
        >
          <NativeSelectOption value=''>
            {t('All categories')}
          </NativeSelectOption>
          {CATEGORIES.map((value) => (
            <NativeSelectOption key={value} value={value}>
              {categoryLabel(value, t)}
            </NativeSelectOption>
          ))}
        </NativeSelect>
        <NativeSelect
          aria-label={t('Result')}
          value={state}
          onChange={(event) => {
            setState(event.target.value)
            firstPage()
          }}
        >
          <NativeSelectOption value=''>{t('All results')}</NativeSelectOption>
          {STATES.map((value) => (
            <NativeSelectOption key={value} value={value}>
              {eventStateLabel(value, t)}
            </NativeSelectOption>
          ))}
        </NativeSelect>
        <NativeSelect
          aria-label={t('Time range')}
          value={days}
          onChange={(event) => {
            const value = event.target.value
            setDays(value)
            setStart(
              value
                ? Math.floor(Date.now() / 1000) - Number(value) * 86400
                : undefined
            )
            firstPage()
          }}
        >
          <NativeSelectOption value=''>{t('All time')}</NativeSelectOption>
          <NativeSelectOption value='1'>
            {t('Last 24 hours')}
          </NativeSelectOption>
          <NativeSelectOption value='7'>{t('Last 7 days')}</NativeSelectOption>
          <NativeSelectOption value='30'>
            {t('Last 30 days')}
          </NativeSelectOption>
        </NativeSelect>
        {props.userId !== undefined && (
          <UserFilterChip userId={props.userId} onClear={props.onClearUser} />
        )}
      </form>
      <div aria-busy={query.isFetching}>
        {query.isLoading && <Skeleton className='h-32 w-full' />}
        {query.isError && (
          <ErrorState
            className='min-h-32'
            title={t('Could not load events')}
            description={query.error.message}
            onRetry={() => void query.refetch()}
          />
        )}
        {query.data && (
          <div className='space-y-3'>
            <EventsTable rows={query.data.items ?? []} onVoid={setVoidTarget} />
            <ServerPagination
              total={query.data.total}
              pagination={pagination}
              onPaginationChange={setPagination}
              settled={query.isSuccess && !query.isFetching}
            />
          </div>
        )}
      </div>
      <VoidEventDialog event={voidTarget} onClose={() => setVoidTarget(null)} />
    </div>
  )
}

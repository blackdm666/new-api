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
import { RefreshCw, Search } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useDebounce } from '@/hooks/use-debounce'
import { formatNumber, formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'

import { AdminUserIdentity } from './admin-user-identity'
import { fetchAdminAffiliateInviteRecords } from './api'
import { formatCents } from './lib'
import type { AdminAffiliateInviteRecord } from './types'

const PAGE_SIZE = 10

export function AdminInviteRecords() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [keyword, setKeyword] = useState('')
  const debouncedKeyword = useDebounce(keyword, 300)
  const params = useMemo(
    () => ({ page, pageSize: PAGE_SIZE, keyword: debouncedKeyword }),
    [debouncedKeyword, page]
  )
  const query = useQuery({
    queryKey: ['admin-affiliate', 'invitees', params],
    queryFn: () => fetchAdminAffiliateInviteRecords(params),
    placeholderData: keepPreviousData,
  })
  const items = query.data?.items ?? []
  const total = query.data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  return (
    <div className='space-y-4'>
      <div className='bg-card flex flex-wrap items-center gap-2 rounded-xl border p-3'>
        <Button variant='outline' onClick={() => void query.refetch()}>
          <RefreshCw
            className={cn('size-4', query.isFetching && 'animate-spin')}
          />
          {t('Refresh')}
        </Button>
        <div className='relative min-w-[280px] flex-1 md:max-w-[560px]'>
          <Label htmlFor='affiliate-invite-search' className='sr-only'>
            {t('Search by promoter, invited user, or UID')}
          </Label>
          <Search
            className='text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2'
            aria-hidden='true'
          />
          <Input
            id='affiliate-invite-search'
            value={keyword}
            onChange={(event) => {
              setKeyword(event.target.value)
              setPage(1)
            }}
            placeholder={t('Search by promoter, invited user, or UID')}
            className='pl-9'
          />
        </div>
      </div>
      <div className='bg-card overflow-x-auto rounded-xl border'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Promoter')}</TableHead>
              <TableHead>{t('Invited user')}</TableHead>
              <TableHead>{t('Invited At')}</TableHead>
              <TableHead className='text-right'>{t('Top-ups')}</TableHead>
              <TableHead className='text-right'>{t('Top-up amount')}</TableHead>
              <TableHead className='text-right'>
                {t('Generated commission')}
              </TableHead>
              <TableHead>{t('Last top-up')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {query.isLoading ? <InviteRecordSkeletonRows /> : null}
            {!query.isLoading && items.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={7}
                  className='text-muted-foreground py-12 text-center'
                >
                  {t('No invitation records')}
                </TableCell>
              </TableRow>
            ) : null}
            {!query.isLoading
              ? items.map((item) => (
                  <AdminInviteRecordRow key={item.invitee_id} item={item} />
                ))
              : null}
          </TableBody>
        </Table>
      </div>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <span className='text-muted-foreground text-xs'>
          {t('Total {{n}} records', { n: total })}
        </span>
        <div className='flex items-center gap-2'>
          <Button
            variant='outline'
            size='sm'
            disabled={query.isLoading || page <= 1}
            onClick={() => setPage((value) => Math.max(1, value - 1))}
          >
            {t('Previous')}
          </Button>
          <span className='text-muted-foreground text-xs'>
            {t('Page {{p}} / {{total}}', { p: page, total: totalPages })}
          </span>
          <Button
            variant='outline'
            size='sm'
            disabled={query.isLoading || page >= totalPages}
            onClick={() => setPage((value) => Math.min(totalPages, value + 1))}
          >
            {t('Next')}
          </Button>
        </div>
      </div>
    </div>
  )
}

export function AdminInviteRecordRow(props: {
  item: AdminAffiliateInviteRecord
}) {
  const { t } = useTranslation()
  const item = props.item
  return (
    <TableRow>
      <TableCell>
        <AdminUserIdentity
          id={item.inviter_id}
          username={item.inviter_username}
        />
      </TableCell>
      <TableCell>
        <div className='flex items-center gap-2'>
          <AdminUserIdentity
            id={item.invitee_id}
            username={item.invitee_username}
          />
          {item.is_new ? <Badge variant='secondary'>{t('New')}</Badge> : null}
        </div>
      </TableCell>
      <TableCell className='text-muted-foreground text-xs whitespace-nowrap'>
        {formatTimestampToDate(item.created_at)}
      </TableCell>
      <TableCell className='text-right tabular-nums'>
        {formatNumber(item.topup_count)}
      </TableCell>
      <TableCell className='text-right font-medium'>
        {formatCents(item.topup_amount_cents)}
      </TableCell>
      <TableCell className='text-right font-medium'>
        {formatCents(item.commission_cents)}
      </TableCell>
      <TableCell className='text-muted-foreground text-xs whitespace-nowrap'>
        {item.last_topup_time > 0
          ? formatTimestampToDate(item.last_topup_time)
          : '-'}
      </TableCell>
    </TableRow>
  )
}

function InviteRecordSkeletonRows() {
  return ['one', 'two', 'three', 'four', 'five'].map((row) => (
    <TableRow key={row}>
      {Array.from({ length: 7 }, (_, index) => (
        <TableCell key={`${row}-${index}`}>
          <Skeleton className='h-4 w-full' />
        </TableCell>
      ))}
    </TableRow>
  ))
}

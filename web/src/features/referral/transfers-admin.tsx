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
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

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
import { formatQuota, formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'

import { AdminUserIdentity } from './admin-user-identity'
import { fetchAdminAffiliateTransfers } from './api'
import { formatCents } from './lib'
import type { AffiliateTransfer } from './types'

const PAGE_SIZE = 10

export function AffiliateTransferManagement() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [keyword, setKeyword] = useState('')
  const debouncedKeyword = useDebounce(keyword, 300)
  const params = useMemo(
    () => ({ page, pageSize: PAGE_SIZE, keyword: debouncedKeyword }),
    [debouncedKeyword, page]
  )
  const query = useQuery({
    queryKey: ['admin-affiliate', 'transfers', params],
    queryFn: () => fetchAdminAffiliateTransfers(params),
    placeholderData: keepPreviousData,
  })
  useEffect(() => setPage(1), [debouncedKeyword])

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
        <div className='relative min-w-[280px] flex-1 md:max-w-[600px]'>
          <Label htmlFor='affiliate-transfer-search' className='sr-only'>
            {t('Search by user, UID, record ID, or request ID')}
          </Label>
          <Search
            className='text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2'
            aria-hidden='true'
          />
          <Input
            id='affiliate-transfer-search'
            value={keyword}
            onChange={(event) => setKeyword(event.target.value)}
            placeholder={t('Search by user, UID, record ID, or request ID')}
            className='pl-9'
          />
        </div>
      </div>
      <AffiliateTransferTable
        items={query.data?.items ?? []}
        loading={query.isLoading}
      />
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

export function AffiliateTransferTable(props: {
  items: AffiliateTransfer[]
  loading: boolean
}) {
  const { t } = useTranslation()
  return (
    <div className='bg-card overflow-x-auto rounded-xl border'>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>ID</TableHead>
            <TableHead>{t('User')}</TableHead>
            <TableHead className='text-right'>{t('Transfer amount')}</TableHead>
            <TableHead>{t('Commission balance change')}</TableHead>
            <TableHead>{t('API balance change')}</TableHead>
            <TableHead>{t('Created at')}</TableHead>
            <TableHead>{t('Request ID')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.loading ? <TransferSkeletonRows /> : null}
          {!props.loading && props.items.length === 0 ? (
            <TableRow>
              <TableCell
                colSpan={7}
                className='text-muted-foreground py-12 text-center'
              >
                {t('No balance transfer records')}
              </TableCell>
            </TableRow>
          ) : null}
          {!props.loading
            ? props.items.map((item) => (
                <TableRow key={item.id}>
                  <TableCell className='font-mono'>{item.id}</TableCell>
                  <TableCell>
                    <AdminUserIdentity
                      id={item.user_id}
                      username={item.username}
                    />
                  </TableCell>
                  <TableCell className='text-right font-semibold tabular-nums'>
                    {formatCents(item.amount_cents)}
                  </TableCell>
                  <TableCell className='whitespace-nowrap tabular-nums'>
                    {formatCents(item.balance_cents_before)} →{' '}
                    {formatCents(item.balance_cents_after)}
                  </TableCell>
                  <TableCell className='whitespace-nowrap tabular-nums'>
                    {formatQuota(item.quota_before)} →{' '}
                    {formatQuota(item.quota_after)}
                  </TableCell>
                  <TableCell className='text-muted-foreground text-xs whitespace-nowrap'>
                    {formatTimestampToDate(item.created_time)}
                  </TableCell>
                  <TableCell
                    className='max-w-56 truncate font-mono text-xs'
                    title={item.request_id}
                  >
                    {item.request_id}
                  </TableCell>
                </TableRow>
              ))
            : null}
        </TableBody>
      </Table>
    </div>
  )
}

function TransferSkeletonRows() {
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

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

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
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
import { formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'

import { AdminUserIdentity } from './admin-user-identity'
import { fetchAdminAffiliateCommissions } from './api'
import {
  AFFILIATE_STATUS_META,
  formatCents,
  formatRate,
  promoterTierBadgeClassName,
  promoterTierLabelKey,
} from './lib'
import { OverflowNote } from './overflow-note'
import type { AffiliateCommission, AffiliateCommissionStatus } from './types'

const PAGE_SIZE = 10

export function AffiliateCommissionLedger() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [status, setStatus] = useState<'all' | AffiliateCommissionStatus>('all')
  const [keyword, setKeyword] = useState('')
  const debouncedKeyword = useDebounce(keyword, 300)
  const params = useMemo(
    () => ({ page, pageSize: PAGE_SIZE, status, keyword: debouncedKeyword }),
    [debouncedKeyword, page, status]
  )
  const query = useQuery({
    queryKey: ['admin-affiliate', 'ledger', params],
    queryFn: () => fetchAdminAffiliateCommissions(params),
    placeholderData: keepPreviousData,
  })
  useEffect(() => setPage(1), [debouncedKeyword, status])

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
        <Label htmlFor='affiliate-ledger-status' className='sr-only'>
          {t('Filter by status')}
        </Label>
        <Select
          value={String(status)}
          onValueChange={(value) =>
            setStatus(
              value === 'all'
                ? 'all'
                : (Number(value) as AffiliateCommissionStatus)
            )
          }
        >
          <SelectTrigger id='affiliate-ledger-status' className='w-[150px]'>
            <SelectValue>
              {status === 'all'
                ? t('All statuses')
                : t(AFFILIATE_STATUS_META[status].labelKey)}
            </SelectValue>
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            <SelectItem value='all'>{t('All statuses')}</SelectItem>
            {Object.entries(AFFILIATE_STATUS_META).map(([value, meta]) => (
              <SelectItem key={value} value={value}>
                {t(meta.labelKey)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <div className='relative min-w-[280px] flex-1 md:max-w-[560px]'>
          <Label htmlFor='affiliate-ledger-search' className='sr-only'>
            {t('Search by order number, promoter, or invited user')}
          </Label>
          <Search
            className='text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2'
            aria-hidden='true'
          />
          <Input
            id='affiliate-ledger-search'
            value={keyword}
            onChange={(event) => setKeyword(event.target.value)}
            placeholder={t('Search by order number, promoter, or invited user')}
            className='pl-9'
          />
        </div>
      </div>
      <div className='bg-card overflow-x-auto rounded-xl border'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>ID</TableHead>
              <TableHead>{t('Promoter')}</TableHead>
              <TableHead>{t('Invited user')}</TableHead>
              <TableHead>{t('Order No.')}</TableHead>
              <TableHead className='text-right'>{t('Top-up amount')}</TableHead>
              <TableHead>{t('Rate')}</TableHead>
              <TableHead>{t('Promoter tier')}</TableHead>
              <TableHead className='text-right'>{t('Commission')}</TableHead>
              <TableHead>{t('Status')}</TableHead>
              <TableHead>{t('Created at')}</TableHead>
              <TableHead>{t('Updated at')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {query.isLoading ? <LedgerSkeletonRows /> : null}
            {!query.isLoading && items.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={11}
                  className='text-muted-foreground py-12 text-center'
                >
                  {t('No commission ledger entries')}
                </TableCell>
              </TableRow>
            ) : null}
            {!query.isLoading
              ? items.map((item) => (
                  <CommissionLedgerRow key={item.id} item={item} />
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

export function CommissionLedgerRow(props: { item: AffiliateCommission }) {
  const { t } = useTranslation()
  const meta = AFFILIATE_STATUS_META[props.item.status]
  return (
    <TableRow>
      <TableCell className='font-mono'>{props.item.id}</TableCell>
      <TableCell>
        <AdminUserIdentity
          id={props.item.inviter_id}
          username={props.item.inviter_username}
        />
      </TableCell>
      <TableCell>
        <AdminUserIdentity
          id={props.item.invitee_id}
          username={props.item.invitee_username}
        />
      </TableCell>
      <TableCell className='font-mono text-xs'>{props.item.trade_no}</TableCell>
      <TableCell className='text-right font-medium'>
        {formatCents(props.item.topup_amount_cents)}
      </TableCell>
      <TableCell>{formatRate(props.item.rate_basis_points)}</TableCell>
      <TableCell>
        <Badge
          variant='outline'
          className={cn(
            'font-normal',
            promoterTierBadgeClassName(props.item.tier_name)
          )}
        >
          {t(promoterTierLabelKey(props.item.tier_name))}
        </Badge>
      </TableCell>
      <TableCell className='text-right font-semibold'>
        {formatCents(props.item.commission_cents)}
      </TableCell>
      <TableCell>
        <div className='flex min-w-0 items-center gap-2'>
          <Badge
            variant='outline'
            className={cn('shrink-0 font-normal', meta.className)}
          >
            {t(meta.labelKey)}
          </Badge>
          {props.item.reject_reason ? (
            <OverflowNote
              text={props.item.reject_reason}
              className='text-muted-foreground max-w-48 text-xs'
            />
          ) : null}
        </div>
      </TableCell>
      <TableCell className='text-muted-foreground text-xs whitespace-nowrap'>
        {formatTimestampToDate(props.item.created_time)}
      </TableCell>
      <TableCell className='text-muted-foreground text-xs whitespace-nowrap'>
        {formatTimestampToDate(props.item.updated_time)}
      </TableCell>
    </TableRow>
  )
}

function LedgerSkeletonRows() {
  return ['one', 'two', 'three', 'four', 'five'].map((row) => (
    <TableRow key={row}>
      {Array.from({ length: 11 }, (_, index) => (
        <TableCell key={`${row}-${index}`}>
          <Skeleton className='h-4 w-full' />
        </TableCell>
      ))}
    </TableRow>
  ))
}

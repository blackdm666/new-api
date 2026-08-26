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
import {
  keepPreviousData,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatTimestampToDate } from '@/lib/format'
import { getUserFacingErrorMessage } from '@/lib/user-facing-error'
import { cn } from '@/lib/utils'

import { cancelAffiliatePayout, fetchAffiliatePayouts } from './api'
import { AFFILIATE_PAYOUT_STATUS_META, formatCents } from './lib'
import {
  AFFILIATE_PAYOUT_STATUS,
  type AffiliatePayout,
  type AffiliatePayoutStatus,
} from './types'

const PAGE_SIZE = 10

export function AffiliatePayouts() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [status, setStatus] = useState<'all' | AffiliatePayoutStatus>('all')
  const [cancelTarget, setCancelTarget] = useState<AffiliatePayout | null>(null)
  const [cancelling, setCancelling] = useState(false)
  const listQuery = useQuery({
    queryKey: ['referral', 'payouts', page, status],
    queryFn: () => fetchAffiliatePayouts(page, PAGE_SIZE, status),
    placeholderData: keepPreviousData,
  })
  const items = listQuery.data?.items ?? []
  const totalPages = Math.max(
    1,
    Math.ceil((listQuery.data?.total ?? 0) / PAGE_SIZE)
  )

  const refresh = async () => {
    await queryClient.invalidateQueries({ queryKey: ['referral'] })
  }
  const cancel = async () => {
    if (!cancelTarget) return
    setCancelling(true)
    try {
      await cancelAffiliatePayout(cancelTarget.id)
      toast.success(t('Payout application cancelled'))
      setCancelTarget(null)
      await refresh()
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setCancelling(false)
    }
  }

  return (
    <div className='space-y-4'>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <div>
          <h3 className='text-sm font-medium'>{t('Payout applications')}</h3>
          <p className='text-muted-foreground text-xs'>
            {t('Review payout status and cancel pending applications.')}
          </p>
        </div>
        <Select
          value={String(status)}
          onValueChange={(value) => {
            setPage(1)
            setStatus(
              value === 'all' ? 'all' : (Number(value) as AffiliatePayoutStatus)
            )
          }}
        >
          <SelectTrigger className='w-[170px]'>
            <SelectValue>
              {status === 'all'
                ? t('All statuses')
                : t(AFFILIATE_PAYOUT_STATUS_META[status].labelKey)}
            </SelectValue>
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            <SelectItem value='all'>{t('All statuses')}</SelectItem>
            {Object.entries(AFFILIATE_PAYOUT_STATUS_META).map(
              ([value, meta]) => (
                <SelectItem key={value} value={value}>
                  {t(meta.labelKey)}
                </SelectItem>
              )
            )}
          </SelectContent>
        </Select>
      </div>
      <div className='overflow-x-auto rounded-xl border'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Amount')}</TableHead>
              <TableHead>{t('Payment method')}</TableHead>
              <TableHead>{t('Payment account')}</TableHead>
              <TableHead>{t('Status')}</TableHead>
              <TableHead>{t('Scheduled payout date')}</TableHead>
              <TableHead>{t('Submitted at')}</TableHead>
              <TableHead className='text-right'>{t('Actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {!listQuery.isLoading && items.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={7}
                  className='text-muted-foreground py-12 text-center'
                >
                  {t('No payout applications')}
                </TableCell>
              </TableRow>
            ) : null}
            {items.map((item) => (
              <PayoutRow
                key={item.id}
                item={item}
                onCancel={() => setCancelTarget(item)}
              />
            ))}
          </TableBody>
        </Table>
      </div>
      <div className='flex items-center justify-end gap-2'>
        <Button
          variant='outline'
          size='sm'
          disabled={page <= 1 || listQuery.isFetching}
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
          disabled={page >= totalPages || listQuery.isFetching}
          onClick={() => setPage((value) => Math.min(totalPages, value + 1))}
        >
          {t('Next')}
        </Button>
      </div>
      <ConfirmDialog
        open={cancelTarget !== null}
        onOpenChange={(open) => {
          if (!open && !cancelling) setCancelTarget(null)
        }}
        title={t('Cancel payout application')}
        desc={t(
          'The reserved commission will be returned to your available commission balance.'
        )}
        confirmText={t('Cancel application')}
        destructive
        isLoading={cancelling}
        handleConfirm={() => void cancel()}
      />
    </div>
  )
}

function PayoutRow({
  item,
  onCancel,
}: {
  item: AffiliatePayout
  onCancel: () => void
}) {
  const { t } = useTranslation()
  const meta = AFFILIATE_PAYOUT_STATUS_META[item.status]
  return (
    <TableRow>
      <TableCell className='font-semibold'>
        {formatCents(item.amount_cents)}
      </TableCell>
      <TableCell>
        {item.payment_method === 'alipay' ? t('Alipay') : t('Bank transfer')}
      </TableCell>
      <TableCell>
        <div className='font-medium'>{item.account_name}</div>
        <div className='text-muted-foreground max-w-56 truncate text-xs'>
          {item.account}
        </div>
      </TableCell>
      <TableCell>
        <div className='flex min-w-0 items-center gap-2'>
          <Badge variant='outline' className={cn('shrink-0', meta.className)}>
            {t(meta.labelKey)}
          </Badge>
          {item.reject_reason ? (
            <span className='text-destructive max-w-48 truncate text-xs'>
              {item.reject_reason}
            </span>
          ) : null}
        </div>
      </TableCell>
      <TableCell className='text-xs'>
        {formatTimestampToDate(item.eligible_settlement_time)}
      </TableCell>
      <TableCell className='text-muted-foreground text-xs'>
        {formatTimestampToDate(item.created_time)}
      </TableCell>
      <TableCell className='text-right'>
        {item.status === AFFILIATE_PAYOUT_STATUS.PENDING ? (
          <Button variant='outline' size='sm' onClick={onCancel}>
            {t('Cancel application')}
          </Button>
        ) : (
          '-'
        )}
      </TableCell>
    </TableRow>
  )
}

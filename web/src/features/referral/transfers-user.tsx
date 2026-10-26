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
import { Button } from '@/components/ui/button'
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

import { cancelAffiliateTransfer, fetchAffiliateTransfers } from './api'
import { formatCents } from './lib'
import { TransferStatus } from './transfers-admin'
import { AFFILIATE_TRANSFER_STATUS, type AffiliateTransfer } from './types'

const PAGE_SIZE = 10

export function AffiliateTransfers() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [cancelTarget, setCancelTarget] = useState<AffiliateTransfer | null>(
    null
  )
  const [cancelling, setCancelling] = useState(false)
  const listQuery = useQuery({
    queryKey: ['referral', 'transfers', page],
    queryFn: () => fetchAffiliateTransfers(page, PAGE_SIZE),
    placeholderData: keepPreviousData,
  })
  const items = listQuery.data?.items ?? []
  const totalPages = Math.max(
    1,
    Math.ceil((listQuery.data?.total ?? 0) / PAGE_SIZE)
  )

  const cancel = async () => {
    if (!cancelTarget) return
    setCancelling(true)
    try {
      await cancelAffiliateTransfer(cancelTarget.id)
      toast.success(t('Balance transfer cancelled'))
      setCancelTarget(null)
      await queryClient.invalidateQueries({ queryKey: ['referral'] })
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setCancelling(false)
    }
  }

  return (
    <div className='space-y-4'>
      <div>
        <h3 className='text-sm font-medium'>{t('Balance transfer records')}</h3>
        <p className='text-muted-foreground text-xs'>
          {t(
            'Transfers under review keep the commission frozen. You can cancel a transfer before it is reviewed.'
          )}
        </p>
      </div>
      <div className='overflow-x-auto rounded-xl border'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Transfer amount')}</TableHead>
              <TableHead>{t('Status')}</TableHead>
              <TableHead>{t('Submitted at')}</TableHead>
              <TableHead>{t('Reviewed at')}</TableHead>
              <TableHead className='text-right'>{t('Actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {!listQuery.isLoading && items.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={5}
                  className='text-muted-foreground py-12 text-center'
                >
                  {t('No balance transfer records')}
                </TableCell>
              </TableRow>
            ) : null}
            {items.map((item) => (
              <TableRow key={item.id}>
                <TableCell className='font-semibold tabular-nums'>
                  {formatCents(item.amount_cents)}
                </TableCell>
                <TableCell>
                  <TransferStatus item={item} />
                </TableCell>
                <TableCell className='text-muted-foreground text-xs'>
                  {formatTimestampToDate(item.created_time)}
                </TableCell>
                <TableCell className='text-muted-foreground text-xs'>
                  {item.reviewed_time
                    ? formatTimestampToDate(item.reviewed_time)
                    : '-'}
                </TableCell>
                <TableCell className='text-right'>
                  {item.status === AFFILIATE_TRANSFER_STATUS.PENDING ? (
                    <Button
                      variant='outline'
                      size='sm'
                      onClick={() => setCancelTarget(item)}
                    >
                      {t('Cancel transfer')}
                    </Button>
                  ) : (
                    '-'
                  )}
                </TableCell>
              </TableRow>
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
        title={t('Cancel balance transfer')}
        desc={t(
          'The frozen commission will be returned to your available commission balance.'
        )}
        confirmText={t('Cancel transfer')}
        destructive
        isLoading={cancelling}
        handleConfirm={() => void cancel()}
      />
    </div>
  )
}

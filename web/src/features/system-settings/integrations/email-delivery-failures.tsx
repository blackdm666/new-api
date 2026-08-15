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
import { RefreshCw, RotateCcw } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { api } from '@/lib/api'
import { formatTimestampToDate } from '@/lib/format'
import { getUserFacingErrorMessage } from '@/lib/user-facing-error'
import { cn } from '@/lib/utils'

const PAGE_SIZE = 10

type EmailDeliveryFailure = {
  id: number
  category: string
  recipient: string
  attempts: number
  last_error: string
  dead_letter_time: number
}

type EmailDeliveryFailurePage = {
  items: EmailDeliveryFailure[]
  total: number
}

const CATEGORY_LABELS: Record<string, string> = {
  email_verification: 'Email verification',
  password_reset: 'Password reset email',
  user_notification: 'System notification email',
  system_alert: 'System notification email',
  quota_warning_user: 'Quota warning email',
  channel_status_admin: 'Channel status email',
  inspection_alert_admin: 'Inspection alert email',
  affiliate_upgrade_admin: 'Promoter upgrade eligibility notification',
  affiliate_upgrade_user: 'Promoter tier upgrade notification',
  affiliate_commission_user: 'Commission review result notification',
  affiliate_payout_user: 'Commission payout status notification',
}

async function fetchFailures(page: number): Promise<EmailDeliveryFailurePage> {
  const response = await api.get('/api/option/email_deliveries/failures', {
    params: {
      p: page,
      page_size: PAGE_SIZE,
      cache_bust: Date.now(),
    },
    disableDuplicate: true,
    skipErrorHandler: true,
  })
  if (!response.data?.success) {
    throw new Error(response.data?.message || 'Failed to load email deliveries')
  }
  return {
    items: response.data.data?.items ?? [],
    total: response.data.data?.total ?? 0,
  }
}

export function EmailDeliveryFailures() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [retryingId, setRetryingId] = useState(0)
  const query = useQuery({
    queryKey: ['email-delivery-failures', page],
    queryFn: () => fetchFailures(page),
    placeholderData: keepPreviousData,
  })
  const total = query.data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  const retry = async (id: number) => {
    if (retryingId > 0) return
    setRetryingId(id)
    try {
      const response = await api.post(
        `/api/option/email_deliveries/${id}/retry`
      )
      if (!response.data?.success) {
        throw new Error(response.data?.message || t('Failed to retry email'))
      }
      toast.success(t('Email retry scheduled'))
      await query.refetch()
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setRetryingId(0)
    }
  }

  return (
    <div className='border-border/70 space-y-4 border-t pt-6'>
      <div className='flex flex-wrap items-start justify-between gap-3'>
        <div>
          <h3 className='text-base font-semibold'>
            {t('Email delivery failures')}
          </h3>
          <p className='text-muted-foreground mt-1 text-sm'>
            {t(
              'System emails retry automatically. Messages shown here exhausted all attempts and can be retried after SMTP is fixed.'
            )}
          </p>
        </div>
        <Button
          type='button'
          variant='outline'
          size='sm'
          onClick={() => void query.refetch()}
          disabled={query.isFetching}
        >
          <RefreshCw
            className={cn('size-4', query.isFetching && 'animate-spin')}
          />
          {t('Refresh')}
        </Button>
      </div>

      <div className='overflow-hidden rounded-xl border'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Email type')}</TableHead>
              <TableHead>{t('Recipient')}</TableHead>
              <TableHead>{t('Attempts')}</TableHead>
              <TableHead>{t('Last error')}</TableHead>
              <TableHead>{t('Failed at')}</TableHead>
              <TableHead className='text-right'>{t('Actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {query.isError ? (
              <TableRow>
                <TableCell
                  colSpan={6}
                  className='text-destructive h-24 text-center'
                >
                  {t('Failed to load email delivery records')}
                </TableCell>
              </TableRow>
            ) : null}
            {query.data?.items.map((item) => (
              <TableRow key={item.id}>
                <TableCell>
                  {t(CATEGORY_LABELS[item.category] || 'System email')}
                </TableCell>
                <TableCell>{item.recipient}</TableCell>
                <TableCell>{item.attempts}</TableCell>
                <TableCell className='max-w-80 whitespace-normal'>
                  <span className='line-clamp-2' title={item.last_error}>
                    {item.last_error || '-'}
                  </span>
                </TableCell>
                <TableCell>
                  {formatTimestampToDate(item.dead_letter_time)}
                </TableCell>
                <TableCell className='text-right'>
                  <Button
                    type='button'
                    size='sm'
                    variant='outline'
                    disabled={retryingId > 0}
                    onClick={() => void retry(item.id)}
                  >
                    <RotateCcw
                      className={cn(
                        'size-4',
                        retryingId === item.id && 'animate-spin'
                      )}
                    />
                    {t('Retry')}
                  </Button>
                </TableCell>
              </TableRow>
            ))}
            {!query.isLoading &&
            !query.isError &&
            (query.data?.items.length ?? 0) === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={6}
                  className='text-muted-foreground h-24 text-center'
                >
                  {t('No failed email deliveries')}
                </TableCell>
              </TableRow>
            ) : null}
          </TableBody>
        </Table>
      </div>

      <div className='flex items-center justify-between gap-3'>
        <span className='text-muted-foreground text-sm'>
          {t('{{count}} records', { count: total })}
        </span>
        <div className='flex items-center gap-2'>
          <Button
            type='button'
            variant='outline'
            size='sm'
            disabled={page <= 1}
            onClick={() => setPage((value) => Math.max(1, value - 1))}
          >
            {t('Previous')}
          </Button>
          <span className='text-muted-foreground text-sm tabular-nums'>
            {page} / {totalPages}
          </span>
          <Button
            type='button'
            variant='outline'
            size='sm'
            disabled={page >= totalPages}
            onClick={() => setPage((value) => Math.min(totalPages, value + 1))}
          >
            {t('Next')}
          </Button>
        </div>
      </div>
    </div>
  )
}

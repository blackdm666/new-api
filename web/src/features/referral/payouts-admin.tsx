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
import {
  Banknote,
  Check,
  ChevronDown,
  RefreshCw,
  Search,
  WalletCards,
  X,
} from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
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
import { Textarea } from '@/components/ui/textarea'
import { useDebounce } from '@/hooks/use-debounce'
import { formatNumber, formatTimestampToDate } from '@/lib/format'
import { getUserFacingErrorMessage } from '@/lib/user-facing-error'
import { cn } from '@/lib/utils'

import {
  approveAffiliatePayout,
  fetchAffiliateAlipayPayoutProviderStatus,
  fetchAdminAffiliatePayouts,
  fetchAdminAffiliatePayoutSummary,
  markAffiliatePayoutPaid,
  payAffiliatePayoutWithAlipay,
  refreshAffiliatePayoutAlipayStatus,
  rejectAffiliatePayout,
} from './api'
import { AFFILIATE_PAYOUT_STATUS_META, formatCents } from './lib'
import { OverflowNote } from './overflow-note'
import {
  AFFILIATE_PAYOUT_STATUS,
  type AffiliatePayout,
  type AffiliatePayoutStatus,
} from './types'

const PAGE_SIZE = 10

export type ReviewAction = 'approve' | 'reject' | 'manual' | 'alipay'

export function AffiliatePayoutManagement() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [status, setStatus] = useState<'all' | AffiliatePayoutStatus>('all')
  const [keyword, setKeyword] = useState('')
  const [target, setTarget] = useState<{
    item: AffiliatePayout
    action: ReviewAction
  } | null>(null)
  const [reason, setReason] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [refreshingId, setRefreshingId] = useState<number | null>(null)
  const debouncedKeyword = useDebounce(keyword, 300)
  const params = useMemo(
    () => ({
      page,
      pageSize: PAGE_SIZE,
      status,
      keyword: debouncedKeyword,
    }),
    [debouncedKeyword, page, status]
  )
  const summaryQuery = useQuery({
    queryKey: ['admin-affiliate-payouts', 'summary'],
    queryFn: fetchAdminAffiliatePayoutSummary,
  })
  const listQuery = useQuery({
    queryKey: ['admin-affiliate-payouts', 'list', params],
    queryFn: () => fetchAdminAffiliatePayouts(params),
    placeholderData: keepPreviousData,
  })
  const providerQuery = useQuery({
    queryKey: ['admin-affiliate-payout-provider'],
    queryFn: fetchAffiliateAlipayPayoutProviderStatus,
  })
  useEffect(() => setPage(1), [status, debouncedKeyword])

  const items = listQuery.data?.items ?? []
  const total = listQuery.data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))
  const closeDialog = () => {
    setTarget(null)
    setReason('')
  }
  const submitAction = async () => {
    if (!target) return
    setSubmitting(true)
    try {
      if (target.action === 'approve') {
        await approveAffiliatePayout(target.item.id)
        toast.success(t('Payout approved'))
      } else if (target.action === 'reject') {
        await rejectAffiliatePayout(target.item.id, reason.trim())
        toast.success(t('Payout rejected'))
      } else if (target.action === 'manual') {
        await markAffiliatePayoutPaid(target.item.id)
        toast.success(t('Payout marked as paid'))
      } else {
        const payout = await payAffiliatePayoutWithAlipay(target.item.id)
        toast.success(
          payout.status === AFFILIATE_PAYOUT_STATUS.PAID
            ? t('Alipay payout completed')
            : t(
                'Alipay payout submitted; use status refresh before trying again'
              )
        )
      }
      closeDialog()
      await queryClient.invalidateQueries({
        queryKey: ['admin-affiliate-payouts'],
      })
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setSubmitting(false)
    }
  }
  const actionDisabled =
    submitting || (target?.action === 'reject' && !reason.trim())
  let dialogTitle = t('Confirm manual payout')
  let dialogConfirmText = t('Confirm paid manually')
  if (target?.action === 'approve') {
    dialogTitle = t('Approve payout')
    dialogConfirmText = t('Approve')
  } else if (target?.action === 'reject') {
    dialogTitle = t('Reject payout')
    dialogConfirmText = t('Reject')
  } else if (target?.action === 'alipay') {
    dialogTitle = t('Confirm Alipay direct payout')
    dialogConfirmText = t('Pay with Alipay')
  }
  const refreshAlipayStatus = async (item: AffiliatePayout) => {
    setRefreshingId(item.id)
    try {
      const payout = await refreshAffiliatePayoutAlipayStatus(item.id)
      toast.success(
        payout.status === AFFILIATE_PAYOUT_STATUS.PAID
          ? t('Alipay payout completed')
          : t('Alipay is still processing this payout')
      )
      await queryClient.invalidateQueries({
        queryKey: ['admin-affiliate-payouts'],
      })
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setRefreshingId(null)
    }
  }

  return (
    <Card data-card-hover='false'>
      <CardContent className='space-y-4'>
        <div>
          <h2 className='font-semibold'>{t('Payout management')}</h2>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Review applications at any time. The 10th of each month is the planned payout date; record payment after the actual transfer is completed.'
            )}
          </p>
        </div>
        <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-6'>
          <PayoutAdminStat
            label={t('Pending review')}
            value={formatNumber(summaryQuery.data?.pending_count ?? 0)}
          />
          <PayoutAdminStat
            label={t('Approved for payout')}
            value={formatNumber(summaryQuery.data?.approved_count ?? 0)}
          />
          <PayoutAdminStat
            label={t('Pending amount')}
            value={formatCents(summaryQuery.data?.pending_cents ?? 0)}
          />
          <PayoutAdminStat
            label={t('Approved amount')}
            value={formatCents(summaryQuery.data?.approved_cents ?? 0)}
          />
          <PayoutAdminStat
            label={t('Alipay processing amount')}
            value={formatCents(summaryQuery.data?.processing_cents ?? 0)}
          />
          <PayoutAdminStat
            label={t('Paid amount')}
            value={formatCents(summaryQuery.data?.paid_cents ?? 0)}
          />
        </div>
        <div className='flex flex-wrap items-center gap-2'>
          <Select
            value={String(status)}
            onValueChange={(value) =>
              setStatus(
                value === 'all'
                  ? 'all'
                  : (Number(value) as AffiliatePayoutStatus)
              )
            }
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
          <div className='relative min-w-[280px] flex-1 md:max-w-[620px]'>
            <Label htmlFor='payout-search' className='sr-only'>
              {t(
                'Search by user, account name, application ID, or payment reference'
              )}
            </Label>
            <Search className='text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2' />
            <Input
              id='payout-search'
              value={keyword}
              onChange={(event) => setKeyword(event.target.value)}
              className='pl-9'
              placeholder={t(
                'Search by user, account name, application ID, or payment reference'
              )}
            />
          </div>
        </div>
        <div className='overflow-x-auto rounded-lg border'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>ID</TableHead>
                <TableHead>{t('User')}</TableHead>
                <TableHead>{t('Amount')}</TableHead>
                <TableHead>{t('Payment information')}</TableHead>
                <TableHead>{t('Status')}</TableHead>
                <TableHead>{t('Scheduled payout date')}</TableHead>
                <TableHead>{t('Submitted at')}</TableHead>
                <TableHead className='w-[150px] min-w-[150px] text-center whitespace-nowrap'>
                  {t('Actions')}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {!listQuery.isLoading && items.length === 0 ? (
                <TableRow>
                  <TableCell
                    colSpan={8}
                    className='text-muted-foreground py-10 text-center'
                  >
                    {t('No payout applications')}
                  </TableCell>
                </TableRow>
              ) : null}
              {items.map((item) => (
                <PayoutAdminRow
                  key={item.id}
                  item={item}
                  onAction={(action) => {
                    setReason('')
                    setTarget({ item, action })
                  }}
                  directPayoutAvailable={
                    providerQuery.data?.enabled === true &&
                    providerQuery.data.configured === true
                  }
                  settlementOpen={
                    summaryQuery.data?.is_settlement_day === true &&
                    Math.floor(Date.now() / 1000) >=
                      item.eligible_settlement_time
                  }
                  refreshing={refreshingId === item.id}
                  onRefresh={() => void refreshAlipayStatus(item)}
                />
              ))}
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
              onClick={() =>
                setPage((value) => Math.min(totalPages, value + 1))
              }
            >
              {t('Next')}
            </Button>
          </div>
        </div>
      </CardContent>
      <ConfirmDialog
        open={target !== null}
        onOpenChange={(open) => {
          if (!open && !submitting) closeDialog()
        }}
        title={dialogTitle}
        desc={
          target ? (
            <div className='space-y-2'>
              <p>
                {t('Payout application #{{id}}, amount {{amount}}.', {
                  id: target.item.id,
                  amount: formatCents(target.item.amount_cents),
                })}
              </p>
              <p>
                {t('Recipient: {{name}} · {{account}}', {
                  name: target.item.account_name,
                  account: target.item.account,
                })}
              </p>
              {target.action === 'manual' ? (
                <p>
                  {t(
                    'Only confirm after completing the transfer outside the system. No transaction number is required, and the system records an internal audit reference.'
                  )}
                </p>
              ) : null}
              {target.action === 'alipay' ? (
                <p>
                  {t(
                    'The system will pay this account directly through Alipay. An uncertain response stays in processing and must be queried before another attempt.'
                  )}
                </p>
              ) : null}
            </div>
          ) : (
            ''
          )
        }
        confirmText={dialogConfirmText}
        destructive={target?.action === 'reject'}
        isLoading={submitting}
        disabled={actionDisabled}
        handleConfirm={() => void submitAction()}
      >
        {target?.action === 'reject' ? (
          <div className='space-y-2'>
            <Label htmlFor='payout-reject-reason'>
              {t('Rejection reason')}
            </Label>
            <Textarea
              id='payout-reject-reason'
              value={reason}
              onChange={(event) => setReason(event.target.value)}
              maxLength={500}
            />
          </div>
        ) : null}
      </ConfirmDialog>
    </Card>
  )
}

function PayoutAdminStat({ label, value }: { label: string; value: string }) {
  return (
    <div className='bg-muted/30 rounded-lg border p-3'>
      <p className='text-muted-foreground text-xs'>{label}</p>
      <p className='mt-1 font-semibold tabular-nums'>{value}</p>
    </div>
  )
}

export function PayoutAdminRow(props: {
  item: AffiliatePayout
  onAction: (action: ReviewAction) => void
  directPayoutAvailable: boolean
  settlementOpen: boolean
  refreshing: boolean
  onRefresh: () => void
}) {
  const { t } = useTranslation()
  const meta = AFFILIATE_PAYOUT_STATUS_META[props.item.status]
  const providerErrorMessage =
    props.item.provider_error_code ===
      'RESPONSE_SIGNATURE_VERIFICATION_FAILED' ||
    props.item.provider_error_message ===
      'response signature verification failed'
      ? t(
          'Alipay response verification failed. Check that the App ID and Alipay public certificate belong to the same application.'
        )
      : props.item.provider_error_message
  return (
    <TableRow>
      <TableCell>{props.item.id}</TableCell>
      <TableCell>
        <div className='font-medium'>
          {props.item.display_name || props.item.username || props.item.user_id}
        </div>
        <div className='text-muted-foreground text-xs'>
          UID {props.item.user_id} · {props.item.username}
        </div>
      </TableCell>
      <TableCell className='font-semibold'>
        {formatCents(props.item.amount_cents)}
      </TableCell>
      <TableCell>
        <div className='font-medium'>{props.item.account_name}</div>
        <div className='text-muted-foreground text-xs'>
          {props.item.payment_method === 'alipay'
            ? t('Alipay')
            : t('Bank transfer')}{' '}
          · {props.item.account}
        </div>
        {props.item.payment_reference ? (
          <div className='text-muted-foreground text-xs'>
            {t('Payout reference')}: {props.item.payment_reference}
          </div>
        ) : null}
        {props.item.provider_order_id ? (
          <div className='text-muted-foreground text-xs'>
            {t('Alipay order number')}: {props.item.provider_order_id}
          </div>
        ) : null}
      </TableCell>
      <TableCell className='min-w-48 align-middle'>
        <div className='flex min-w-0 flex-col items-start gap-1.5'>
          <Badge variant='outline' className={cn('shrink-0', meta.className)}>
            {t(meta.labelKey)}
          </Badge>
          {providerErrorMessage ? (
            <OverflowNote
              text={providerErrorMessage}
              className='text-destructive max-w-56 text-xs leading-4'
            />
          ) : null}
        </div>
      </TableCell>
      <TableCell className='text-xs'>
        {formatTimestampToDate(props.item.eligible_settlement_time)}
      </TableCell>
      <TableCell className='text-muted-foreground text-xs'>
        {formatTimestampToDate(props.item.created_time)}
      </TableCell>
      <TableCell className='w-[150px] min-w-[150px] text-center align-middle whitespace-nowrap'>
        {props.item.status === AFFILIATE_PAYOUT_STATUS.PENDING ? (
          <div className='flex flex-nowrap justify-center gap-2'>
            <Button size='sm' onClick={() => props.onAction('approve')}>
              <Check className='size-4' />
              {t('Approve')}
            </Button>
            <Button
              size='sm'
              variant='destructive'
              onClick={() => props.onAction('reject')}
            >
              <X className='size-4' />
              {t('Reject')}
            </Button>
          </div>
        ) : null}
        {props.item.status === AFFILIATE_PAYOUT_STATUS.APPROVED ? (
          <DropdownMenu>
            <DropdownMenuTrigger
              render={<Button size='sm' disabled={!props.settlementOpen} />}
              title={
                props.settlementOpen
                  ? undefined
                  : t('Payouts can only be settled on the scheduled payout day')
              }
            >
              <WalletCards className='size-4' />
              {t('Settle payout')}
              <ChevronDown className='size-3.5 opacity-70' />
            </DropdownMenuTrigger>
            <DropdownMenuContent align='end' className='w-40'>
              <DropdownMenuItem onClick={() => props.onAction('manual')}>
                <Banknote className='size-4' />
                {t('Manual payout')}
              </DropdownMenuItem>
              <DropdownMenuItem
                disabled={!props.directPayoutAvailable}
                title={
                  props.directPayoutAvailable
                    ? undefined
                    : t('Configure and enable Alipay direct payout first')
                }
                onClick={() => props.onAction('alipay')}
              >
                <WalletCards className='size-4' />
                {t('Alipay direct payout')}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        ) : null}
        {props.item.status === AFFILIATE_PAYOUT_STATUS.PROCESSING ? (
          <Button
            size='sm'
            variant='outline'
            disabled={props.refreshing}
            onClick={props.onRefresh}
          >
            <RefreshCw
              className={cn('size-4', props.refreshing && 'animate-spin')}
            />
            {t('Refresh payout status')}
          </Button>
        ) : null}
      </TableCell>
    </TableRow>
  )
}

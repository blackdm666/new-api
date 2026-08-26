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
import { Check, RefreshCw, Search, X } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { useDebounce } from '@/hooks/use-debounce'
import { formatNumber, formatTimestampToDate } from '@/lib/format'
import { getUserFacingErrorMessage } from '@/lib/user-facing-error'
import { cn } from '@/lib/utils'

import { AdminUserIdentity } from './admin-user-identity'
import {
  approveAffiliateCommission,
  approveAffiliateUpgrade,
  fetchAdminAffiliateCommissions,
  fetchAdminAffiliateSummary,
  fetchAffiliateNotificationFailures,
  fetchAffiliateUpgradeCandidates,
  rejectAffiliateCommission,
  retryAffiliateNotification,
} from './api'
import { AffiliateCommissionLedger } from './commission-ledger-admin'
import { AdminInviteRecords } from './invite-records-admin'
import {
  AFFILIATE_STATUS_META,
  formatCents,
  formatRate,
  promoterTierBadgeClassName,
  promoterTierLabelKey,
} from './lib'
import { OverflowNote } from './overflow-note'
import { AffiliatePayoutManagement } from './payouts-admin'
import { AffiliateTransferManagement } from './transfers-admin'
import { AFFILIATE_COMMISSION_STATUS, type AffiliateCommission } from './types'

const PAGE_SIZE = 10

export function AdminAffiliatePage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [keyword, setKeyword] = useState(() => {
    if (typeof window === 'undefined') return ''
    return new URLSearchParams(window.location.search).get('keyword') ?? ''
  })
  const [reviewTarget, setReviewTarget] = useState<{
    item: AffiliateCommission
    approve: boolean
  } | null>(null)
  const [reviewing, setReviewing] = useState(false)
  const [rejectReason, setRejectReason] = useState('')
  const [upgradingId, setUpgradingId] = useState<number | null>(null)
  const [retryingNoticeId, setRetryingNoticeId] = useState<number | null>(null)
  const debouncedKeyword = useDebounce(keyword, 300)
  const params = useMemo(
    () => ({
      page,
      pageSize: PAGE_SIZE,
      status: AFFILIATE_COMMISSION_STATUS.PENDING,
      keyword: debouncedKeyword,
    }),
    [debouncedKeyword, page]
  )
  const summaryQuery = useQuery({
    queryKey: ['admin-affiliate', 'summary'],
    queryFn: fetchAdminAffiliateSummary,
  })
  const listQuery = useQuery({
    queryKey: ['admin-affiliate', 'commissions', params],
    queryFn: () => fetchAdminAffiliateCommissions(params),
    placeholderData: keepPreviousData,
  })
  const upgradeQuery = useQuery({
    queryKey: ['admin-affiliate', 'upgrade-candidates'],
    queryFn: () => fetchAffiliateUpgradeCandidates(),
  })
  const notificationFailureQuery = useQuery({
    queryKey: ['admin-affiliate', 'notification-failures'],
    queryFn: () => fetchAffiliateNotificationFailures(),
  })
  useEffect(() => setPage(1), [debouncedKeyword])

  const refresh = () => {
    void summaryQuery.refetch()
    void listQuery.refetch()
    void upgradeQuery.refetch()
    void notificationFailureQuery.refetch()
  }

  const review = async (item: AffiliateCommission, approve: boolean) => {
    setReviewing(true)
    try {
      if (approve) {
        await approveAffiliateCommission(item.id)
        toast.success(t('Commission approved'))
      } else {
        await rejectAffiliateCommission(item.id, rejectReason.trim())
        toast.success(t('Commission rejected'))
      }
      await queryClient.invalidateQueries({ queryKey: ['admin-affiliate'] })
      setReviewTarget(null)
      setRejectReason('')
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setReviewing(false)
    }
  }

  const approveUpgrade = async (inviterId: number, nextGroup: string) => {
    setUpgradingId(inviterId)
    try {
      await approveAffiliateUpgrade(inviterId, nextGroup)
      toast.success(t('Promoter upgraded'))
      await queryClient.invalidateQueries({ queryKey: ['admin-affiliate'] })
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setUpgradingId(null)
    }
  }

  const retryNotice = async (id: number) => {
    setRetryingNoticeId(id)
    try {
      await retryAffiliateNotification(id)
      toast.success(t('Notification queued for retry'))
      await notificationFailureQuery.refetch()
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setRetryingNoticeId(null)
    }
  }

  const total = listQuery.data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))
  const items = listQuery.data?.items ?? []
  return (
    <div className='h-full overflow-y-auto px-4 py-6 sm:px-8'>
      <div className='mx-auto w-full max-w-[1440px] space-y-5'>
        <header>
          <h1 className='text-xl font-semibold tracking-tight'>
            {t('Affiliate Management')}
          </h1>
          <p className='text-muted-foreground mt-1 text-sm'>
            {t(
              'View all invitation relationships and manage promoter commission operations.'
            )}
          </p>
        </header>
        <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-5'>
          <AdminStat
            label={t('Pending review')}
            value={formatNumber(summaryQuery.data?.pending_count ?? 0)}
          />
          <AdminStat
            label={t('Pending commission')}
            value={formatCents(summaryQuery.data?.pending_cents ?? 0)}
          />
          <AdminStat
            label={t('Approved commission')}
            value={formatCents(summaryQuery.data?.approved_cents ?? 0)}
          />
          <AdminStat
            label={t('Total invited users')}
            value={formatNumber(summaryQuery.data?.total_invitee_count ?? 0)}
          />
          <AdminStat
            label={t('Effective top-up users')}
            value={formatNumber(
              summaryQuery.data?.effective_invitee_count ?? 0
            )}
          />
        </div>
        <Tabs defaultValue='invitees' className='gap-4'>
          <TabsList className='max-w-full flex-wrap justify-start group-data-horizontal/tabs:h-auto'>
            <TabsTrigger value='invitees'>
              {t('Invitation records')}
            </TabsTrigger>
            <TabsTrigger value='ledger'>{t('Commission ledger')}</TabsTrigger>
            <TabsTrigger value='commissions'>
              {t('Commission review')}
            </TabsTrigger>
            <TabsTrigger value='settlement'>
              {t('Commission settlement')}
            </TabsTrigger>
            <TabsTrigger value='upgrades'>{t('Upgrade review')}</TabsTrigger>
          </TabsList>
          <TabsContent value='invitees'>
            <AdminInviteRecords />
          </TabsContent>
          <TabsContent value='ledger'>
            <AffiliateCommissionLedger />
          </TabsContent>
          <TabsContent value='settlement'>
            <Tabs defaultValue='payouts' className='gap-4'>
              <TabsList variant='line'>
                <TabsTrigger value='payouts'>
                  {t('Payout applications')}
                </TabsTrigger>
                <TabsTrigger value='transfers'>
                  {t('Balance transfer records')}
                </TabsTrigger>
              </TabsList>
              <TabsContent value='payouts'>
                <AffiliatePayoutManagement />
              </TabsContent>
              <TabsContent value='transfers'>
                <AffiliateTransferManagement />
              </TabsContent>
            </Tabs>
          </TabsContent>
          <TabsContent value='upgrades' className='space-y-5'>
            <Card data-card-hover='false'>
              <CardContent className='space-y-3'>
                <div>
                  <h2 className='font-semibold'>
                    {t('Promoter upgrade review')}
                  </h2>
                  <p className='text-muted-foreground text-sm'>
                    {t(
                      'Promoters shown here reached either the people or top-up amount criterion and can be upgraded after review.'
                    )}
                  </p>
                </div>
                <div className='overflow-x-auto rounded-lg border'>
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>{t('Promoter')}</TableHead>
                        <TableHead>{t('Current tier')}</TableHead>
                        <TableHead>{t('Upgrade criteria')}</TableHead>
                        <TableHead>{t('Target tier')}</TableHead>
                        <TableHead className='text-right'>
                          {t('Actions')}
                        </TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {(upgradeQuery.data?.items ?? []).map((item) => (
                        <TableRow key={item.inviter_id}>
                          <TableCell>
                            <AdminUserIdentity
                              id={item.inviter_id}
                              username={item.username}
                            />
                          </TableCell>
                          <TableCell>
                            <Badge
                              variant='outline'
                              className={cn(
                                'font-normal',
                                promoterTierBadgeClassName(item.current_group)
                              )}
                            >
                              {t(promoterTierLabelKey(item.current_group))}
                            </Badge>
                          </TableCell>
                          <TableCell>
                            <div className='space-y-1 text-xs tabular-nums'>
                              <div
                                className={cn(
                                  'flex items-center gap-1.5',
                                  item.eligible_by_invitees &&
                                    'text-emerald-600 dark:text-emerald-400'
                                )}
                              >
                                {item.eligible_by_invitees ? (
                                  <Check className='size-3.5' />
                                ) : null}
                                <span>
                                  {t('Effective top-up users')}:{' '}
                                  <strong>
                                    {item.effective_invitee_count}/
                                    {item.threshold}
                                  </strong>
                                </span>
                              </div>
                              <div
                                className={cn(
                                  'flex items-center gap-1.5',
                                  item.eligible_by_top_up_amount &&
                                    'text-emerald-600 dark:text-emerald-400'
                                )}
                              >
                                {item.eligible_by_top_up_amount ? (
                                  <Check className='size-3.5' />
                                ) : null}
                                <span>
                                  {t('Effective top-up amount')}:{' '}
                                  <strong>
                                    {formatCents(
                                      item.effective_top_up_amount_cents
                                    )}
                                    /
                                    {formatCents(
                                      item.top_up_amount_threshold_cents
                                    )}
                                  </strong>
                                </span>
                              </div>
                            </div>
                          </TableCell>
                          <TableCell>
                            <Badge
                              variant='outline'
                              className={cn(
                                'font-normal',
                                promoterTierBadgeClassName(item.next_group)
                              )}
                            >
                              {t(promoterTierLabelKey(item.next_group))} ·{' '}
                              {formatRate(item.next_rate_basis_points)}
                            </Badge>
                          </TableCell>
                          <TableCell className='text-right'>
                            <Button
                              size='sm'
                              disabled={upgradingId !== null}
                              onClick={() =>
                                void approveUpgrade(
                                  item.inviter_id,
                                  item.next_group
                                )
                              }
                            >
                              <Check className='size-4' />
                              {upgradingId === item.inviter_id
                                ? t('Upgrading')
                                : t('Approve upgrade')}
                            </Button>
                          </TableCell>
                        </TableRow>
                      ))}
                      {!upgradeQuery.isLoading &&
                      (upgradeQuery.data?.items.length ?? 0) === 0 ? (
                        <TableRow>
                          <TableCell
                            colSpan={5}
                            className='text-muted-foreground py-8 text-center'
                          >
                            {t('No promoters awaiting upgrade')}
                          </TableCell>
                        </TableRow>
                      ) : null}
                    </TableBody>
                  </Table>
                </div>
              </CardContent>
            </Card>
            {(notificationFailureQuery.data?.total ?? 0) > 0 ? (
              <Card data-card-hover='false'>
                <CardContent className='space-y-3'>
                  <div>
                    <h2 className='font-semibold'>
                      {t('Upgrade notification failures')}
                    </h2>
                    <p className='text-muted-foreground text-sm'>
                      {t(
                        'Failed upgrade emails use exponential backoff and enter the failure list after repeated errors.'
                      )}
                    </p>
                  </div>
                  <div className='overflow-x-auto rounded-lg border'>
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead>{t('Promoter')}</TableHead>
                          <TableHead>{t('Attempts')}</TableHead>
                          <TableHead>{t('Last error')}</TableHead>
                          <TableHead>{t('Next retry')}</TableHead>
                          <TableHead className='text-right'>
                            {t('Actions')}
                          </TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {(notificationFailureQuery.data?.items ?? []).map(
                          (item) => (
                            <TableRow key={item.id}>
                              <TableCell>
                                <AdminUserIdentity
                                  id={item.inviter_id}
                                  username={item.inviter_username}
                                />
                              </TableCell>
                              <TableCell>{item.attempt_count}</TableCell>
                              <TableCell className='max-w-md text-xs break-words'>
                                {item.last_error}
                              </TableCell>
                              <TableCell className='text-xs'>
                                {item.dead_letter_time > 0
                                  ? t('Manual retry required')
                                  : formatTimestampToDate(
                                      item.next_attempt_time
                                    )}
                              </TableCell>
                              <TableCell className='text-right'>
                                <Button
                                  variant='outline'
                                  size='sm'
                                  disabled={retryingNoticeId !== null}
                                  onClick={() => void retryNotice(item.id)}
                                >
                                  {retryingNoticeId === item.id
                                    ? t('Retrying')
                                    : t('Retry now')}
                                </Button>
                              </TableCell>
                            </TableRow>
                          )
                        )}
                      </TableBody>
                    </Table>
                  </div>
                </CardContent>
              </Card>
            ) : null}
          </TabsContent>
          <TabsContent value='commissions' className='space-y-4'>
            <div className='bg-card flex flex-wrap items-center gap-2 rounded-xl border p-3'>
              <Button variant='outline' onClick={refresh}>
                <RefreshCw
                  className={cn(
                    'size-4',
                    (summaryQuery.isFetching || listQuery.isFetching) &&
                      'animate-spin'
                  )}
                />
                {t('Refresh')}
              </Button>
              <div className='relative min-w-[280px] flex-1 md:max-w-[560px]'>
                <Label htmlFor='affiliate-search' className='sr-only'>
                  {t('Search by order number, promoter, or invited user')}
                </Label>
                <Search
                  className='text-muted-foreground pointer-events-none absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2'
                  aria-hidden='true'
                />
                <Input
                  id='affiliate-search'
                  value={keyword}
                  onChange={(event) => setKeyword(event.target.value)}
                  placeholder={t(
                    'Search by order number, promoter, or invited user'
                  )}
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
                    <TableHead className='text-right'>
                      {t('Top-up amount')}
                    </TableHead>
                    <TableHead>{t('Rate')}</TableHead>
                    <TableHead>{t('Promoter tier')}</TableHead>
                    <TableHead className='text-right'>
                      {t('Commission')}
                    </TableHead>
                    <TableHead>{t('Status')}</TableHead>
                    <TableHead>{t('Created at')}</TableHead>
                    <TableHead className='text-right'>{t('Actions')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {listQuery.isLoading ? <SkeletonRows /> : null}
                  {!listQuery.isLoading && items.length === 0 ? (
                    <TableRow>
                      <TableCell
                        colSpan={11}
                        className='text-muted-foreground py-12 text-center'
                      >
                        {t('No commission records')}
                      </TableCell>
                    </TableRow>
                  ) : null}
                  {!listQuery.isLoading
                    ? items.map((item) => (
                        <AdminCommissionRow
                          key={item.id}
                          item={item}
                          onApprove={() =>
                            setReviewTarget({ item, approve: true })
                          }
                          onReject={() => {
                            setRejectReason('')
                            setReviewTarget({ item, approve: false })
                          }}
                        />
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
                  disabled={listQuery.isLoading || page <= 1}
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
                  disabled={listQuery.isLoading || page >= totalPages}
                  onClick={() =>
                    setPage((value) => Math.min(totalPages, value + 1))
                  }
                >
                  {t('Next')}
                </Button>
              </div>
            </div>
          </TabsContent>
        </Tabs>
      </div>
      <ConfirmDialog
        open={reviewTarget !== null}
        onOpenChange={(open) => {
          if (!open && !reviewing) setReviewTarget(null)
        }}
        title={
          reviewTarget?.approve
            ? t('Approve commission')
            : t('Reject commission')
        }
        desc={
          reviewTarget
            ? t(
                'Order {{order}} will be marked as {{status}}. Commission: {{amount}}.',
                {
                  order: reviewTarget.item.trade_no,
                  status: reviewTarget.approve ? t('Approved') : t('Rejected'),
                  amount: formatCents(reviewTarget.item.commission_cents),
                }
              )
            : ''
        }
        confirmText={reviewTarget?.approve ? t('Approve') : t('Reject')}
        destructive={reviewTarget?.approve === false}
        isLoading={reviewing}
        disabled={
          reviewing || (reviewTarget?.approve === false && !rejectReason.trim())
        }
        handleConfirm={() => {
          if (reviewTarget) {
            void review(reviewTarget.item, reviewTarget.approve)
          }
        }}
      >
        {reviewTarget?.approve === false ? (
          <div className='space-y-2'>
            <Label htmlFor='affiliate-reject-reason'>
              {t('Rejection reason')}
            </Label>
            <Textarea
              id='affiliate-reject-reason'
              value={rejectReason}
              onChange={(event) => setRejectReason(event.target.value)}
              maxLength={255}
              placeholder={t('Explain why this commission was rejected')}
            />
          </div>
        ) : null}
      </ConfirmDialog>
    </div>
  )
}

function AdminStat({ label, value }: { label: string; value: string }) {
  return (
    <Card data-card-hover='false' className='bg-muted/20'>
      <CardContent>
        <p className='text-muted-foreground text-sm'>{label}</p>
        <p className='mt-1 text-2xl font-semibold tabular-nums'>{value}</p>
      </CardContent>
    </Card>
  )
}

function AdminCommissionRow(props: {
  item: AffiliateCommission
  onApprove: () => void
  onReject: () => void
}) {
  const { t } = useTranslation()
  const meta = AFFILIATE_STATUS_META[props.item.status]
  const pending = props.item.status === AFFILIATE_COMMISSION_STATUS.PENDING
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
      <TableCell className='text-right'>
        <span className='font-semibold'>
          {formatCents(props.item.commission_cents)}
        </span>
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
      <TableCell className='text-muted-foreground text-xs'>
        {formatTimestampToDate(props.item.created_time)}
      </TableCell>
      <TableCell className='text-right'>
        <div className='flex justify-end gap-2'>
          <Button size='sm' disabled={!pending} onClick={props.onApprove}>
            <Check className='size-4' />
            {t('Approve')}
          </Button>
          <Button
            size='sm'
            variant='outline'
            disabled={!pending}
            onClick={props.onReject}
          >
            <X className='size-4' />
            {t('Reject')}
          </Button>
        </div>
      </TableCell>
    </TableRow>
  )
}

function SkeletonRows() {
  const columns = [
    'promoter',
    'invitee',
    'order',
    'topup',
    'rate',
    'tier',
    'commission',
    'status',
    'created',
    'reviewed',
    'actions',
  ]
  return ['one', 'two', 'three', 'four', 'five'].map((row) => (
    <TableRow key={row}>
      {columns.map((column) => (
        <TableCell key={`${row}-${column}`}>
          <Skeleton className='h-4 w-full' />
        </TableCell>
      ))}
    </TableRow>
  ))
}

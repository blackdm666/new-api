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
import { Textarea } from '@/components/ui/textarea'
import { useDebounce } from '@/hooks/use-debounce'
import { formatTimestampToDate } from '@/lib/format'
import { getUserFacingErrorMessage } from '@/lib/user-facing-error'
import { cn } from '@/lib/utils'

import { AdminUserIdentity } from './admin-user-identity'
import {
  approveAffiliateTransfer,
  fetchAdminAffiliateTransfers,
  refreshAffiliateTransferRisk,
  rejectAffiliateTransfer,
} from './api'
import {
  AFFILIATE_RISK_LEVEL_META,
  AFFILIATE_RISK_SIGNAL_LABEL_KEYS,
  affiliateRiskLevel,
  formatCents,
} from './lib'
import {
  AFFILIATE_TRANSFER_STATUS,
  type AffiliateRiskPair,
  type AffiliateTransfer,
} from './types'

const PAGE_SIZE = 10
const SHOWN_PAIRS = 3

/**
 * Review queue for commission-to-balance transfers. Each row carries the
 * requester's self-referral score so the reviewer can decide at a glance.
 */
export function AffiliateTransferReview() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [keyword, setKeyword] = useState('')
  const [reviewTarget, setReviewTarget] = useState<{
    item: AffiliateTransfer
    approve: boolean
  } | null>(null)
  const [reviewing, setReviewing] = useState(false)
  const [rejectReason, setRejectReason] = useState('')
  const [rescoringId, setRescoringId] = useState<number | null>(null)
  const debouncedKeyword = useDebounce(keyword, 300)
  const params = useMemo(
    () => ({
      page,
      pageSize: PAGE_SIZE,
      status: AFFILIATE_TRANSFER_STATUS.PENDING,
      keyword: debouncedKeyword,
    }),
    [debouncedKeyword, page]
  )
  const query = useQuery({
    queryKey: ['admin-affiliate', 'transfer-review', params],
    queryFn: () => fetchAdminAffiliateTransfers(params),
    placeholderData: keepPreviousData,
  })
  useEffect(() => setPage(1), [debouncedKeyword])

  const total = query.data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))
  const items = query.data?.items ?? []

  const review = async (item: AffiliateTransfer, approve: boolean) => {
    setReviewing(true)
    try {
      if (approve) {
        await approveAffiliateTransfer(item.id)
        toast.success(t('Balance transfer approved'))
      } else {
        await rejectAffiliateTransfer(item.id, rejectReason.trim())
        toast.success(t('Balance transfer rejected'))
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

  const rescore = async (id: number) => {
    setRescoringId(id)
    try {
      await refreshAffiliateTransferRisk(id)
      await query.refetch()
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setRescoringId(null)
    }
  }

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
          <Label htmlFor='affiliate-transfer-review-search' className='sr-only'>
            {t('Search by user, UID, record ID, or request ID')}
          </Label>
          <Search
            className='text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2'
            aria-hidden='true'
          />
          <Input
            id='affiliate-transfer-review-search'
            value={keyword}
            onChange={(event) => setKeyword(event.target.value)}
            placeholder={t('Search by user, UID, record ID, or request ID')}
            className='pl-9'
          />
        </div>
        <p className='text-muted-foreground w-full text-xs'>
          {t(
            'Self-referral score: 5 or more is high risk, 3–4 is medium. Commission stays frozen until you approve or reject the transfer.'
          )}
        </p>
      </div>
      <div className='bg-card overflow-x-auto rounded-xl border'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>ID</TableHead>
              <TableHead>{t('Promoter')}</TableHead>
              <TableHead className='text-right'>
                {t('Transfer amount')}
              </TableHead>
              <TableHead>{t('Self-referral score')}</TableHead>
              <TableHead className='min-w-[320px]'>
                {t('Risk evidence')}
              </TableHead>
              <TableHead>{t('Created at')}</TableHead>
              <TableHead className='text-right'>{t('Actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {query.isLoading ? <ReviewSkeletonRows /> : null}
            {!query.isLoading && items.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={7}
                  className='text-muted-foreground py-12 text-center'
                >
                  {t('No balance transfers awaiting review')}
                </TableCell>
              </TableRow>
            ) : null}
            {!query.isLoading
              ? items.map((item) => (
                  <TableRow key={item.id} className='align-top'>
                    <TableCell className='font-mono'>{item.id}</TableCell>
                    <TableCell>
                      <AdminUserIdentity
                        id={item.user_id}
                        username={item.username}
                        remark={item.remark}
                      />
                    </TableCell>
                    <TableCell className='text-right font-semibold tabular-nums'>
                      {formatCents(item.amount_cents)}
                    </TableCell>
                    <TableCell>
                      <TransferRiskBadge item={item} />
                    </TableCell>
                    <TableCell className='max-w-[480px] min-w-[320px] whitespace-normal'>
                      <TransferRiskEvidence item={item} />
                    </TableCell>
                    <TableCell className='text-muted-foreground text-xs whitespace-nowrap'>
                      {formatTimestampToDate(item.created_time)}
                    </TableCell>
                    <TableCell className='text-right'>
                      <div className='flex justify-end gap-2'>
                        <Button
                          size='sm'
                          onClick={() =>
                            setReviewTarget({ item, approve: true })
                          }
                        >
                          <Check className='size-4' />
                          {t('Approve')}
                        </Button>
                        <Button
                          size='sm'
                          variant='outline'
                          onClick={() => {
                            setRejectReason('')
                            setReviewTarget({ item, approve: false })
                          }}
                        >
                          <X className='size-4' />
                          {t('Reject')}
                        </Button>
                        <Button
                          size='sm'
                          variant='ghost'
                          disabled={rescoringId !== null}
                          onClick={() => void rescore(item.id)}
                          aria-label={t('Re-score')}
                          title={t('Re-score')}
                        >
                          <RefreshCw
                            className={cn(
                              'size-4',
                              rescoringId === item.id && 'animate-spin'
                            )}
                          />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
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
      <ConfirmDialog
        open={reviewTarget !== null}
        onOpenChange={(open) => {
          if (!open && !reviewing) setReviewTarget(null)
        }}
        title={
          reviewTarget?.approve
            ? t('Approve balance transfer')
            : t('Reject balance transfer')
        }
        desc={
          reviewTarget
            ? t(
                reviewTarget.approve
                  ? '{{amount}} will be added to the API balance of {{user}}.'
                  : '{{amount}} will be returned to the available commission of {{user}}.',
                {
                  amount: formatCents(reviewTarget.item.amount_cents),
                  user:
                    reviewTarget.item.username ||
                    `#${reviewTarget.item.user_id}`,
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
            <Label htmlFor='affiliate-transfer-reject-reason'>
              {t('Rejection reason')}
            </Label>
            <Textarea
              id='affiliate-transfer-reject-reason'
              value={rejectReason}
              onChange={(event) => setRejectReason(event.target.value)}
              maxLength={255}
              placeholder={t('Explain why this balance transfer was rejected')}
            />
          </div>
        ) : null}
      </ConfirmDialog>
    </div>
  )
}

export function TransferRiskBadge(props: { item: AffiliateTransfer }) {
  const { t } = useTranslation()
  if (!props.item.risk_evaluated_time) {
    return <span className='text-muted-foreground text-xs'>{t('Scoring')}</span>
  }
  const score = props.item.risk_score ?? 0
  const meta = AFFILIATE_RISK_LEVEL_META[affiliateRiskLevel(score)]
  return (
    <Badge variant='outline' className={cn('font-normal', meta.className)}>
      <span className='font-semibold tabular-nums'>{score}</span>
      {t(meta.labelKey)}
    </Badge>
  )
}

export function TransferRiskEvidence(props: { item: AffiliateTransfer }) {
  const { t } = useTranslation()
  const risk = props.item.risk
  if (!risk) return <span className='text-muted-foreground text-xs'>—</span>
  const notes: string[] = [
    risk.single_invitee
      ? t('Only one paying invitee')
      : t('{{n}} paying invitees', { n: risk.invitee_count }),
  ]
  if (risk.invitee_cluster_count > 0) {
    notes.push(
      t('{{n}} groups of invitees share IPs', {
        n: risk.invitee_cluster_count,
      })
    )
  }
  if (risk.suspicious_commission_cents > 0) {
    notes.push(
      t('Suspicious commission {{amount}}', {
        amount: formatCents(risk.suspicious_commission_cents),
      })
    )
  }
  return (
    <div className='space-y-1.5 text-xs'>
      <p className='text-muted-foreground'>{notes.join(' · ')}</p>
      {risk.pairs.slice(0, SHOWN_PAIRS).map((pair) => (
        <RiskPairLine key={pair.invitee_id} pair={pair} />
      ))}
      {risk.pairs.length > SHOWN_PAIRS ? (
        <p className='text-muted-foreground'>
          {t('{{n}} more invitees with signals', {
            n: risk.pairs.length - SHOWN_PAIRS,
          })}
        </p>
      ) : null}
    </div>
  )
}

function RiskPairLine(props: { pair: AffiliateRiskPair }) {
  const { t } = useTranslation()
  const pair = props.pair
  const signals = pair.signals.map((signal) => {
    if (signal === 'shared_ip' && pair.shared_ip) {
      return `${t(AFFILIATE_RISK_SIGNAL_LABEL_KEYS[signal])} ${pair.shared_ip}`
    }
    if (signal === 'registered_within_day') {
      return t('Registered {{gap}} apart', {
        gap: formatRegistrationGap(pair.registration_gap_seconds, t),
      })
    }
    return t(AFFILIATE_RISK_SIGNAL_LABEL_KEYS[signal])
  })
  return (
    <div className='flex flex-wrap items-baseline gap-x-1.5'>
      <span className='font-medium'>
        {pair.invitee_username || `#${pair.invitee_id}`}
      </span>
      <span className='text-muted-foreground font-mono'>
        #{pair.invitee_id}
      </span>
      <span className='font-semibold tabular-nums'>+{pair.score}</span>
      <span className='text-muted-foreground'>
        {signals.join(' · ')} · {formatCents(pair.commission_cents)}
      </span>
    </div>
  )
}

function formatRegistrationGap(
  seconds: number,
  t: (key: string, options?: Record<string, unknown>) => string
): string {
  const gap = Math.abs(seconds)
  if (gap < 3600) {
    return t('{{n}} min', { n: Math.max(1, Math.round(gap / 60)) })
  }
  return t('{{n}} h', { n: Math.round(gap / 3600) })
}

function ReviewSkeletonRows() {
  return ['one', 'two', 'three'].map((row) => (
    <TableRow key={row}>
      {Array.from({ length: 7 }, (_, index) => (
        <TableCell key={`${row}-${index}`}>
          <Skeleton className='h-4 w-full' />
        </TableCell>
      ))}
    </TableRow>
  ))
}

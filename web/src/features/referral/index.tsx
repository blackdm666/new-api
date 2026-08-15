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
  CircleDollarSign,
  Copy,
  Hourglass,
  RefreshCw,
  TrendingUp,
  UserRoundCheck,
  Users,
  WalletCards,
} from 'lucide-react'
import { type MouseEvent, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { SectionPageLayout } from '@/components/layout'
import { Badge, badgeVariants } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Progress } from '@/components/ui/progress'
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
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { formatNumber, formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import {
  fetchAffiliateCommissions,
  fetchAffiliateInviteeStats,
  fetchAffiliatePayoutSummary,
  fetchAffiliateSummary,
} from './api'
import {
  CashPayoutDialog,
  CommissionActions,
  TransferBalanceDialog,
} from './commission-actions'
import {
  AFFILIATE_STATUS_META,
  formatCents,
  formatRate,
  promoterTierBadgeClassName,
  promoterTierLabelKey,
} from './lib'
import { OverflowNote } from './overflow-note'
import { AffiliatePayouts } from './payouts-user'
import type {
  AffiliateCommission,
  AffiliateInviteeStats,
  AffiliateSummary,
} from './types'

const PAGE_SIZE = 10

function showUnavailableTierFeedback(event: MouseEvent<HTMLButtonElement>) {
  if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return
  event.currentTarget.animate(
    [
      { transform: 'translateX(0)' },
      { transform: 'translateX(-3px)' },
      { transform: 'translateX(3px)' },
      { transform: 'translateX(-2px)' },
      { transform: 'translateX(2px)' },
      { transform: 'translateX(0)' },
    ],
    { duration: 240, easing: 'ease-out' }
  )
}

export function ReferralProgram() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const currentBalanceQuota = useAuthStore(
    (state) => state.auth.user?.quota ?? 0
  )
  const [tab, setTab] = useState('commissions')
  const [transferDialogOpen, setTransferDialogOpen] = useState(false)
  const [payoutDialogOpen, setPayoutDialogOpen] = useState(false)
  const { copyToClipboard } = useCopyToClipboard()
  const summaryQuery = useQuery({
    queryKey: ['referral', 'summary'],
    queryFn: fetchAffiliateSummary,
  })
  const codeQuery = useQuery({
    queryKey: ['referral', 'code'],
    queryFn: async () => {
      const { getAffiliateCode } = await import('@/features/wallet/api')
      const response = await getAffiliateCode()
      if (!response.success || !response.data) return ''
      return response.data
    },
  })
  const payoutSummaryQuery = useQuery({
    queryKey: ['referral', 'payout-summary'],
    queryFn: fetchAffiliatePayoutSummary,
  })
  const affiliateLink = useMemo(() => {
    if (!codeQuery.data || typeof window === 'undefined') return ''
    return `${window.location.origin}/sign-up?aff=${codeQuery.data}`
  }, [codeQuery.data])
  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: ['referral'] })
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Referral Program')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='w-full space-y-4'>
          <SummaryGrid
            summary={summaryQuery.data}
            loading={summaryQuery.isLoading}
          />
          <CommissionActions
            availableCents={summaryQuery.data?.available_cents ?? 0}
            minimumPayoutCents={payoutSummaryQuery.data?.minimum_cents ?? 10000}
            settlementDay={payoutSummaryQuery.data?.settlement_day ?? 10}
            loading={summaryQuery.isLoading || payoutSummaryQuery.isLoading}
            onTransfer={() => setTransferDialogOpen(true)}
            onPayout={() => setPayoutDialogOpen(true)}
          />
          <Card data-card-hover='false' className='bg-muted/20'>
            <CardContent className='space-y-4'>
              <div className='flex flex-wrap items-center justify-between gap-3'>
                <div>
                  <div className='flex flex-wrap items-center gap-2'>
                    <h2 className='text-base font-semibold'>
                      {t('Your referral link')}
                    </h2>
                  </div>
                  <p className='text-muted-foreground mt-1 text-sm'>
                    {summaryQuery.data?.auto_approve
                      ? t(
                          'Invited users earn commission automatically after a successful Epay top-up. Commission is added to your available balance immediately.'
                        )
                      : t(
                          'Invited users generate pending commission after a successful Epay top-up. Commission is added to your available balance after administrator approval.'
                        )}
                  </p>
                </div>
                <div className='flex items-center gap-2'>
                  <Button
                    variant='outline'
                    size='sm'
                    onClick={refresh}
                    disabled={summaryQuery.isFetching}
                  >
                    <RefreshCw
                      className={cn(
                        'size-4',
                        summaryQuery.isFetching && 'animate-spin'
                      )}
                    />
                    {t('Refresh')}
                  </Button>
                </div>
              </div>
              <div className='flex items-center gap-2'>
                <Input
                  value={affiliateLink}
                  readOnly
                  className='h-11 flex-1 font-mono text-sm'
                />
                <Button
                  variant='outline'
                  size='icon'
                  className='size-11 shrink-0'
                  disabled={!affiliateLink}
                  onClick={() => copyToClipboard(affiliateLink)}
                  aria-label={t('Copy referral link')}
                >
                  <Copy className='size-4' />
                </Button>
              </div>
              <UpgradeProgress summary={summaryQuery.data} />
            </CardContent>
          </Card>

          <Tabs value={tab} onValueChange={setTab}>
            <TabsList>
              <TabsTrigger value='commissions'>
                {t('Commission records')}
              </TabsTrigger>
              <TabsTrigger value='invitees'>{t('Invited users')}</TabsTrigger>
              <TabsTrigger value='payouts'>
                {t('Payout applications')}
              </TabsTrigger>
            </TabsList>
            <TabsContent value='commissions'>
              <CommissionRecords />
            </TabsContent>
            <TabsContent value='invitees'>
              <InviteeStats />
            </TabsContent>
            <TabsContent value='payouts'>
              <AffiliatePayouts />
            </TabsContent>
          </Tabs>
          {transferDialogOpen ? (
            <TransferBalanceDialog
              open={transferDialogOpen}
              onOpenChange={setTransferDialogOpen}
              availableCents={summaryQuery.data?.available_cents ?? 0}
              currentBalanceQuota={currentBalanceQuota}
            />
          ) : null}
          {payoutDialogOpen ? (
            <CashPayoutDialog
              open={payoutDialogOpen}
              onOpenChange={setPayoutDialogOpen}
              availableCents={payoutSummaryQuery.data?.available_cents ?? 0}
              minimumCents={payoutSummaryQuery.data?.minimum_cents ?? 10000}
              nextSettlementTime={
                payoutSummaryQuery.data?.next_settlement_time ?? 0
              }
            />
          ) : null}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

function SummaryGrid({
  summary,
  loading,
}: {
  summary?: AffiliateSummary
  loading: boolean
}) {
  const { t } = useTranslation()
  const stats = [
    {
      label: t('Available commission'),
      value: formatCents(summary?.available_cents ?? 0),
      icon: CircleDollarSign,
    },
    {
      label: t('Pending commission'),
      value: formatCents(summary?.pending_commission_cents ?? 0),
      icon: Hourglass,
    },
    {
      label: t('Total commission'),
      value: formatCents(summary?.approved_commission_cents ?? 0),
      icon: TrendingUp,
    },
    {
      label: t('Invited users'),
      value: formatNumber(summary?.invite_count ?? 0),
      icon: Users,
    },
    {
      label: t('Effective top-up users'),
      value: formatNumber(summary?.effective_invitee_count ?? 0),
      icon: UserRoundCheck,
    },
    {
      label: t('Invitee top-up amount'),
      value: formatCents(summary?.total_topup_cents ?? 0),
      icon: WalletCards,
    },
  ]
  return (
    <Card data-card-hover='false' className='bg-border gap-0 py-0'>
      <CardContent className='p-0'>
        <div className='bg-border grid grid-cols-2 gap-px sm:grid-cols-3'>
          {stats.map((stat) => {
            const Icon = stat.icon
            return (
              <div
                key={stat.label}
                className='bg-card flex min-h-24 items-center justify-between gap-3 px-4 py-3'
              >
                <div className='min-w-0'>
                  <p className='text-muted-foreground text-sm'>{stat.label}</p>
                  {loading ? (
                    <Skeleton className='mt-2 h-7 w-24' />
                  ) : (
                    <p className='mt-1 truncate text-2xl font-semibold tabular-nums'>
                      {stat.value}
                    </p>
                  )}
                </div>
                <div className='bg-primary/10 text-primary flex size-9 shrink-0 items-center justify-center rounded-full'>
                  <Icon className='size-4.5' />
                </div>
              </div>
            )
          })}
        </div>
      </CardContent>
    </Card>
  )
}

function UpgradeProgress({ summary }: { summary?: AffiliateSummary }) {
  const { t } = useTranslation()
  if (!summary?.upgrade_eligible) {
    return (
      <div className='border-border/70 rounded-xl border p-4'>
        <div className='flex items-center gap-3'>
          <div className='flex size-9 shrink-0 items-center justify-center rounded-full bg-emerald-500/10 text-emerald-600'>
            <UserRoundCheck className='size-4' />
          </div>
          <div>
            <p className='text-sm font-medium'>{t('Current tier confirmed')}</p>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Your current promoter tier is managed by administrators. Future top-ups use the current tier rate.'
              )}
            </p>
          </div>
        </div>
        <TierBadges summary={summary} />
      </div>
    )
  }
  const inviteeProgress = Math.round(
    (summary?.upgrade_progress_ratio ?? 0) * 100
  )
  const amountProgress = Math.round(
    (summary?.upgrade_top_up_amount_progress_ratio ?? 0) * 100
  )
  return (
    <div className='border-border/70 rounded-xl border p-4'>
      <div className='mb-2 flex flex-wrap items-center justify-between gap-2'>
        <div>
          <p className='text-sm font-medium'>{t('Upgrade progress')}</p>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Reach either {{count}} effective top-up users or {{amount}} in valid invitee top-ups, then an administrator can approve an upgrade to {{tier}} ({{rate}}).',
              {
                count: summary?.upgrade_threshold ?? 50,
                amount: formatCents(
                  summary?.upgrade_top_up_amount_threshold_cents ?? 200000
                ),
                tier: summary?.next_tier_name
                  ? t(promoterTierLabelKey(summary.next_tier_name))
                  : t('Advanced promoter'),
                rate: formatRate(summary?.next_tier_rate_basis_points ?? 1000),
              }
            )}
          </p>
        </div>
      </div>
      <TierBadges summary={summary} />
      <div className='mt-3 space-y-3'>
        <UpgradeCriterion
          label={t('Effective top-up users')}
          value={`${summary?.effective_invitee_count ?? 0}/${summary?.upgrade_threshold ?? 50}`}
          progress={inviteeProgress}
        />
        <UpgradeCriterion
          label={t('Effective top-up amount')}
          value={`${formatCents(summary?.total_topup_cents ?? 0)}/${formatCents(summary?.upgrade_top_up_amount_threshold_cents ?? 200000)}`}
          progress={amountProgress}
        />
      </div>
    </div>
  )
}

function UpgradeCriterion(props: {
  label: string
  value: string
  progress: number
}) {
  return (
    <div className='space-y-1.5'>
      <div className='flex items-center justify-between gap-3 text-xs'>
        <span className='text-muted-foreground'>{props.label}</span>
        <span className='font-medium tabular-nums'>{props.value}</span>
      </div>
      <Progress value={props.progress} />
    </div>
  )
}

function TierBadges({ summary }: { summary?: AffiliateSummary }) {
  const { t } = useTranslation()
  const currentTier = summary?.tier_name?.trim()
  const activeTier =
    currentTier === '高级推广' || currentTier === '金牌推广'
      ? currentTier
      : '初级推广'
  const tiers = [
    {
      key: '初级推广',
      label: t('Junior promoter'),
      configuredRate:
        summary?.group_rates?.default ??
        summary?.group_rates?.['初级推广'] ??
        summary?.default_rate_basis_points ??
        500,
    },
    {
      key: '高级推广',
      label: t('Advanced promoter'),
      configuredRate: summary?.group_rates?.['高级推广'] ?? 1000,
    },
    {
      key: '金牌推广',
      label: t('Gold promoter'),
      configuredRate: summary?.group_rates?.['金牌推广'] ?? 1500,
    },
  ]

  return (
    <div className='mt-3 mb-3 flex flex-wrap items-center gap-2'>
      <span className='text-muted-foreground inline-flex h-6 items-center text-sm leading-none font-medium'>
        {t('Current tier')}:
      </span>
      {tiers.map((tier) => {
        const isActive = tier.key === activeTier
        const rate = isActive
          ? (summary?.rate_basis_points ?? tier.configuredRate)
          : tier.configuredRate
        const controlClassName = cn(
          badgeVariants({ variant: 'outline' }),
          promoterTierBadgeClassName(tier.key),
          'h-6 px-2.5 py-0 text-sm leading-none font-normal'
        )
        if (!isActive) {
          return (
            <button
              key={tier.key}
              type='button'
              aria-disabled='true'
              onClick={showUnavailableTierFeedback}
              className={cn(
                controlClassName,
                'cursor-pointer opacity-65 transition-opacity hover:opacity-80 active:opacity-60'
              )}
            >
              {tier.label} · {formatRate(rate)}
            </button>
          )
        }
        return (
          <button
            key={tier.key}
            type='button'
            aria-pressed='true'
            aria-label={`${t('Current tier')}: ${tier.label}, ${t('Commission rate')}: ${formatRate(rate)}`}
            className={cn(
              controlClassName,
              'cursor-pointer transition-[transform,filter,box-shadow] hover:brightness-110 active:scale-[0.97] focus-visible:ring-2 focus-visible:ring-ring/50'
            )}
          >
            {tier.label} · {formatRate(rate)}
          </button>
        )
      })}
    </div>
  )
}

function CommissionRecords() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const query = useQuery({
    queryKey: ['referral', 'commissions', page],
    queryFn: () => fetchAffiliateCommissions(page, PAGE_SIZE),
    placeholderData: keepPreviousData,
  })
  const items = query.data?.items ?? []
  return (
    <PaginatedTableShell
      total={query.data?.total ?? 0}
      page={page}
      onPageChange={setPage}
      loading={query.isLoading}
    >
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Order No.')}</TableHead>
            <TableHead>{t('Invited user')}</TableHead>
            <TableHead className='text-right'>{t('Top-up amount')}</TableHead>
            <TableHead>{t('Rate')}</TableHead>
            <TableHead>{t('Promoter tier')}</TableHead>
            <TableHead className='text-right'>{t('Commission')}</TableHead>
            <TableHead>{t('Status')}</TableHead>
            <TableHead>{t('Created at')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {query.isLoading ? <SkeletonRows columns={8} /> : null}
          {!query.isLoading && items.length === 0 ? (
            <EmptyRow columns={8} text={t('No commission records')} />
          ) : null}
          {!query.isLoading
            ? items.map((item) => <CommissionRow key={item.id} item={item} />)
            : null}
        </TableBody>
      </Table>
    </PaginatedTableShell>
  )
}

function CommissionRow({ item }: { item: AffiliateCommission }) {
  const { t } = useTranslation()
  const meta = AFFILIATE_STATUS_META[item.status]
  return (
    <TableRow>
      <TableCell className='font-mono text-xs'>{item.trade_no}</TableCell>
      <TableCell>
        {item.invitee_display_name ||
          item.invitee_username ||
          t('Unavailable user')}
      </TableCell>
      <TableCell className='text-right font-medium'>
        {formatCents(item.topup_amount_cents)}
      </TableCell>
      <TableCell>{formatRate(item.rate_basis_points)}</TableCell>
      <TableCell>
        <Badge
          variant='outline'
          className={cn(
            'font-normal',
            promoterTierBadgeClassName(item.tier_name)
          )}
        >
          {t(promoterTierLabelKey(item.tier_name))}
        </Badge>
      </TableCell>
      <TableCell className='text-right font-semibold'>
        {formatCents(item.commission_cents)}
      </TableCell>
      <TableCell>
        <div className='flex min-w-0 items-center gap-2'>
          <Badge
            variant='outline'
            className={cn('shrink-0 font-normal', meta.className)}
          >
            {t(meta.labelKey)}
          </Badge>
          {item.reject_reason ? (
            <OverflowNote
              text={item.reject_reason}
              className='text-muted-foreground max-w-48 text-xs'
            />
          ) : null}
        </div>
      </TableCell>
      <TableCell className='text-muted-foreground text-xs'>
        {formatTimestampToDate(item.created_time)}
      </TableCell>
    </TableRow>
  )
}

function InviteeStats() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const query = useQuery({
    queryKey: ['referral', 'invitee-stats', page],
    queryFn: () => fetchAffiliateInviteeStats(page, PAGE_SIZE),
    placeholderData: keepPreviousData,
  })
  const items = query.data?.items ?? []
  return (
    <PaginatedTableShell
      total={query.data?.total ?? 0}
      page={page}
      onPageChange={setPage}
      loading={query.isLoading}
    >
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('User')}</TableHead>
            <TableHead>{t('Registered at')}</TableHead>
            <TableHead className='text-right'>{t('Top-up count')}</TableHead>
            <TableHead className='text-right'>
              {t('Cumulative valid top-up amount')}
            </TableHead>
            <TableHead className='text-right'>
              {t('Generated commission')}
            </TableHead>
            <TableHead>{t('Last top-up')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {query.isLoading ? <SkeletonRows columns={6} /> : null}
          {!query.isLoading && items.length === 0 ? (
            <EmptyRow columns={6} text={t('No invitees yet')} />
          ) : null}
          {!query.isLoading
            ? items.map((item) => (
                <InviteeRow key={item.username} item={item} />
              ))
            : null}
        </TableBody>
      </Table>
    </PaginatedTableShell>
  )
}

function InviteeRow({ item }: { item: AffiliateInviteeStats }) {
  return (
    <TableRow>
      <TableCell>
        <span className='block truncate font-medium'>{item.username}</span>
        {item.display_name && item.display_name !== item.username ? (
          <div className='text-muted-foreground truncate text-xs'>
            {item.display_name}
          </div>
        ) : null}
      </TableCell>
      <TableCell className='text-muted-foreground text-xs'>
        {formatTimestampToDate(item.created_at)}
      </TableCell>
      <TableCell className='text-right tabular-nums'>
        {formatNumber(item.topup_count)}
      </TableCell>
      <TableCell className='text-right font-medium'>
        {formatCents(item.topup_amount_cents)}
      </TableCell>
      <TableCell className='text-right font-semibold'>
        {formatCents(item.commission_cents)}
      </TableCell>
      <TableCell className='text-muted-foreground text-xs'>
        {item.last_topup_time > 0
          ? formatTimestampToDate(item.last_topup_time)
          : '-'}
      </TableCell>
    </TableRow>
  )
}

function PaginatedTableShell(props: {
  children: React.ReactNode
  total: number
  page: number
  loading: boolean
  onPageChange: (page: number) => void
}) {
  const { t } = useTranslation()
  const totalPages = Math.max(1, Math.ceil(props.total / PAGE_SIZE))
  return (
    <div className='space-y-3'>
      <div className='bg-card overflow-x-auto rounded-xl border'>
        {props.children}
      </div>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <span className='text-muted-foreground text-xs'>
          {t('Total {{n}} records', { n: props.total })}
        </span>
        <div className='flex items-center gap-2'>
          <Button
            variant='outline'
            size='sm'
            disabled={props.loading || props.page <= 1}
            onClick={() => props.onPageChange(Math.max(1, props.page - 1))}
          >
            {t('Previous')}
          </Button>
          <span className='text-muted-foreground text-xs'>
            {t('Page {{p}} / {{total}}', {
              p: props.page,
              total: totalPages,
            })}
          </span>
          <Button
            variant='outline'
            size='sm'
            disabled={props.loading || props.page >= totalPages}
            onClick={() =>
              props.onPageChange(Math.min(totalPages, props.page + 1))
            }
          >
            {t('Next')}
          </Button>
        </div>
      </div>
    </div>
  )
}

function SkeletonRows({ columns }: { columns: number }) {
  const columnKeys = Array.from(
    { length: columns },
    (_, index) => `column-${index + 1}`
  )
  return ['one', 'two', 'three'].map((row) => (
    <TableRow key={row}>
      {columnKeys.map((column) => (
        <TableCell key={`${row}-${column}`}>
          <Skeleton className='h-4 w-full' />
        </TableCell>
      ))}
    </TableRow>
  ))
}

function EmptyRow({ columns, text }: { columns: number; text: string }) {
  return (
    <TableRow>
      <TableCell
        colSpan={columns}
        className='text-muted-foreground py-12 text-center'
      >
        <EmptyState title={text} />
      </TableCell>
    </TableRow>
  )
}

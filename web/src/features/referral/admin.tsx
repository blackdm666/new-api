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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Check } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { formatNumber, formatTimestampToDate } from '@/lib/format'
import { getUserFacingErrorMessage } from '@/lib/user-facing-error'
import { cn } from '@/lib/utils'

import { AdminUserIdentity } from './admin-user-identity'
import {
  approveAffiliateUpgrade,
  fetchAdminAffiliateSummary,
  fetchAffiliateNotificationFailures,
  fetchAffiliateUpgradeCandidates,
  retryAffiliateNotification,
} from './api'
import { AffiliateCommissionLedger } from './commission-ledger-admin'
import { AdminInviteRecords } from './invite-records-admin'
import {
  formatCents,
  formatRate,
  promoterTierBadgeClassName,
  promoterTierLabelKey,
} from './lib'
import { AffiliatePayoutManagement } from './payouts-admin'
import { AffiliateTransferReview } from './transfer-review-admin'
import { AffiliateTransferManagement } from './transfers-admin'

export function AdminAffiliatePage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [upgradingId, setUpgradingId] = useState<number | null>(null)
  const [retryingNoticeId, setRetryingNoticeId] = useState<number | null>(null)
  const summaryQuery = useQuery({
    queryKey: ['admin-affiliate', 'summary'],
    queryFn: fetchAdminAffiliateSummary,
  })
  const upgradeQuery = useQuery({
    queryKey: ['admin-affiliate', 'upgrade-candidates'],
    queryFn: () => fetchAffiliateUpgradeCandidates(),
  })
  const notificationFailureQuery = useQuery({
    queryKey: ['admin-affiliate', 'notification-failures'],
    queryFn: () => fetchAffiliateNotificationFailures(),
  })
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
            value={formatNumber(summaryQuery.data?.pending_transfer_count ?? 0)}
          />
          <AdminStat
            label={t('Transfers awaiting review')}
            value={formatCents(summaryQuery.data?.pending_transfer_cents ?? 0)}
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
                              remark={item.remark}
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
                                  remark={item.inviter_remark}
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
          <TabsContent value='commissions'>
            <AffiliateTransferReview />
          </TabsContent>
        </Tabs>
      </div>
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

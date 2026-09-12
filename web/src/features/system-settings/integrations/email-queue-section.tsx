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
import { RotateCcw, Search } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
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
import { useDebounce } from '@/hooks/use-debounce'
import { api } from '@/lib/api'
import { formatTimestampToDate } from '@/lib/format'
import { getUserFacingErrorMessage } from '@/lib/user-facing-error'

import { emailCategoryLabel } from './email-category-labels'
import { EmailQueueRulesCard, type EmailQueueRules } from './email-queue-rules'

const PAGE_SIZE = 20

type DeliveryStatus =
  | 'queued'
  | 'sending'
  | 'retrying'
  | 'awaiting_receipt'
  | 'accepted_untracked'
  | 'delivered'
  | 'failed'
  | 'expired'

type EmailDelivery = {
  id: number
  category: string
  user_id: number
  recipient: string
  priority: number
  status: DeliveryStatus
  sender_account_id: number
  sender_account_name: string
  attempts: number
  last_error: string
  next_attempt_time: number
  delivered_time: number
  dead_letter_time: number
  expired_time: number
  created_time: number
}

type EmailDeliveryPage = {
  items: EmailDelivery[]
  total: number
}

type EmailQueueStats = {
  queue: {
    queued: number
    sending: number
    retrying: number
    awaiting_receipt: number
    accepted_untracked_24h: number
    final_delivered_24h: number
    failed: number
    delivered_24h: number
    failed_24h: number
    failure_rate_24h: number
    oldest_pending_time: number
    last_delivered_time: number
    marketing_quota_used_today: number
  }
  categories: string[]
  smtp_configured: boolean
  marketing_daily_limit: number
  marketing_circuit_breaker: {
    paused_campaigns: number
    last_reason: string
  }
  rules: EmailQueueRules
}

const STATUS_META: Record<
  DeliveryStatus,
  { label: string; className: string }
> = {
  queued: { label: 'Queued', className: 'border-slate-500/40 text-slate-500' },
  sending: { label: 'Sending', className: 'border-blue-500/40 text-blue-500' },
  retrying: {
    label: 'Waiting for retry',
    className: 'border-amber-500/40 text-amber-500',
  },
  awaiting_receipt: {
    label: 'Waiting for receipt',
    className: 'border-violet-500/40 text-violet-500',
  },
  accepted_untracked: {
    label: 'SMTP accepted, untracked',
    className: 'border-cyan-500/40 text-cyan-500',
  },
  delivered: {
    label: 'Final delivery confirmed',
    className: 'border-emerald-500/40 text-emerald-500',
  },
  failed: { label: 'Failed', className: 'border-red-500/40 text-red-500' },
  expired: {
    label: 'Expired',
    className: 'border-zinc-500/40 text-zinc-500',
  },
}

async function fetchQueue(
  page: number,
  status: string,
  category: string,
  keyword: string
): Promise<EmailDeliveryPage> {
  const response = await api.get('/api/option/email_deliveries', {
    params: { p: page, page_size: PAGE_SIZE, status, category, keyword },
    disableDuplicate: true,
    skipErrorHandler: true,
  })
  if (!response.data?.success) {
    throw new Error(response.data?.message || 'Failed to load email queue')
  }
  return {
    items: response.data.data?.items ?? [],
    total: response.data.data?.total ?? 0,
  }
}

async function fetchStats(): Promise<EmailQueueStats> {
  const response = await api.get('/api/option/email_deliveries/stats', {
    disableDuplicate: true,
    skipErrorHandler: true,
  })
  if (!response.data?.success) {
    throw new Error(response.data?.message || 'Failed to load email queue')
  }
  return response.data.data as EmailQueueStats
}

export function EmailQueueSection() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [status, setStatus] = useState('')
  const [category, setCategory] = useState('')
  const [keyword, setKeyword] = useState('')
  const [selected, setSelected] = useState<number[]>([])
  const [retrying, setRetrying] = useState(false)
  const debouncedKeyword = useDebounce(keyword, 300)
  const queueQuery = useQuery({
    queryKey: ['email-queue', page, status, category, debouncedKeyword],
    queryFn: () => fetchQueue(page, status, category, debouncedKeyword),
    placeholderData: keepPreviousData,
  })
  const statsQuery = useQuery({
    queryKey: ['email-queue', 'stats'],
    queryFn: fetchStats,
    refetchInterval: 30_000,
  })
  const items = useMemo(
    () => queueQuery.data?.items ?? [],
    [queueQuery.data?.items]
  )
  const total = queueQuery.data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))
  const selectableIds = useMemo(
    () =>
      items.filter((item) => item.status === 'failed').map((item) => item.id),
    [items]
  )

  const refresh = async () => {
    await Promise.all([queueQuery.refetch(), statsQuery.refetch()])
  }

  const retry = async (ids: number[]) => {
    if (ids.length === 0 || retrying) return
    setRetrying(true)
    try {
      if (ids.length === 1) {
        await api.post(`/api/option/email_deliveries/${ids[0]}/retry`)
      } else {
        await api.post('/api/option/email_deliveries/retry', { ids })
      }
      toast.success(t('Email retry scheduled'))
      setSelected([])
      await refresh()
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setRetrying(false)
    }
  }

  const stats = statsQuery.data?.queue
  return (
    <div className='space-y-5'>
      <Card data-card-hover='false'>
        <CardContent className='space-y-4 py-4'>
          <div>
            <p className='text-sm font-medium'>{t('Email queue')}</p>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Monitor delivery, retries, receipts, and final failures for system and marketing emails.'
              )}
            </p>
          </div>
          <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-6'>
            <HealthItem
              label={t('SMTP status')}
              value={
                statsQuery.data?.smtp_configured
                  ? t('Configured')
                  : t('Not configured')
              }
            />
            <HealthItem
              label={t('Last successful delivery')}
              value={
                stats?.last_delivered_time
                  ? formatTimestampToDate(stats.last_delivered_time)
                  : '-'
              }
            />
            <HealthItem
              label={t('Oldest pending email')}
              value={
                stats?.oldest_pending_time
                  ? formatWaitingTime(stats.oldest_pending_time, t)
                  : '-'
              }
            />
            <HealthItem
              label={t('Queue health')}
              value={queueHealthLabel(stats, t)}
            />
            <HealthItem
              label={t('Marketing circuit breaker')}
              value={
                statsQuery.data?.marketing_circuit_breaker.paused_campaigns
                  ? `${t('{{count}} paused campaigns', { count: statsQuery.data.marketing_circuit_breaker.paused_campaigns })}: ${statsQuery.data.marketing_circuit_breaker.last_reason}`
                  : t('Not triggered')
              }
            />
            <HealthItem
              label={t('Marketing daily usage')}
              value={`${stats?.marketing_quota_used_today ?? 0} / ${statsQuery.data?.marketing_daily_limit ?? 0}`}
            />
          </div>
        </CardContent>
      </Card>

      <div className='flex flex-wrap items-center gap-3'>
        <Select
          items={[
            { value: 'all', label: t('All statuses') },
            ...Object.entries(STATUS_META).map(([value, meta]) => ({
              value,
              label: t(meta.label),
            })),
          ]}
          value={status || 'all'}
          onValueChange={(value) => {
            setStatus(value && value !== 'all' ? value : '')
            setPage(1)
            setSelected([])
          }}
        >
          <SelectTrigger className='w-48' aria-label={t('Email status')}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            <SelectGroup>
              <SelectItem value='all'>{t('All statuses')}</SelectItem>
              {Object.entries(STATUS_META).map(([key, meta]) => (
                <SelectItem key={key} value={key}>
                  {t(meta.label)}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
        <Select
          items={[
            { value: 'all', label: t('All email types') },
            ...(statsQuery.data?.categories ?? []).map((item) => ({
              value: item,
              label: emailCategoryLabel(item, t),
            })),
          ]}
          value={category || 'all'}
          onValueChange={(value) => {
            setCategory(value && value !== 'all' ? value : '')
            setPage(1)
            setSelected([])
          }}
        >
          <SelectTrigger className='w-56' aria-label={t('Email type')}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            <SelectGroup>
              <SelectItem value='all'>{t('All email types')}</SelectItem>
              {(statsQuery.data?.categories ?? []).map((item) => (
                <SelectItem key={item} value={item}>
                  {emailCategoryLabel(item, t)}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
        <div className='relative min-w-64 flex-1'>
          <Search className='text-muted-foreground absolute top-1/2 left-3 size-4 -translate-y-1/2' />
          <Input
            value={keyword}
            onChange={(event) => {
              setKeyword(event.target.value)
              setPage(1)
            }}
            placeholder={t(
              'Search by email type, user, recipient, or queue ID'
            )}
            className='pl-9'
          />
        </div>
        {selected.length > 0 ? (
          <Button
            type='button'
            variant='outline'
            disabled={retrying}
            onClick={() => void retry(selected)}
          >
            <RotateCcw className='size-4' />
            {t('Retry selected')} ({selected.length})
          </Button>
        ) : null}
      </div>

      <div className='overflow-hidden rounded-xl border'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className='w-10'>
                <Checkbox
                  checked={
                    selectableIds.length > 0 &&
                    selectableIds.every((id) => selected.includes(id))
                  }
                  onCheckedChange={(checked) =>
                    setSelected(checked ? selectableIds : [])
                  }
                  aria-label={t('Select failed emails')}
                />
              </TableHead>
              <TableHead>{t('Email type')}</TableHead>
              <TableHead>{t('Recipient')}</TableHead>
              <TableHead>{t('Status')}</TableHead>
              <TableHead>{t('Sender account')}</TableHead>
              <TableHead>{t('Attempts')}</TableHead>
              <TableHead>{t('Next retry')}</TableHead>
              <TableHead>{t('Last error')}</TableHead>
              <TableHead>{t('Created at')}</TableHead>
              <TableHead>{t('Completed at')}</TableHead>
              <TableHead className='text-right'>{t('Actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {items.map((item) => {
              const meta = STATUS_META[item.status]
              return (
                <TableRow key={item.id}>
                  <TableCell>
                    <Checkbox
                      checked={selected.includes(item.id)}
                      disabled={item.status !== 'failed'}
                      onCheckedChange={(checked) =>
                        setSelected((current) =>
                          checked
                            ? [...current, item.id]
                            : current.filter((id) => id !== item.id)
                        )
                      }
                      aria-label={t('Select email #{{id}}', {
                        id: item.id,
                      })}
                    />
                  </TableCell>
                  <TableCell>
                    <div className='font-medium'>
                      {emailCategoryLabel(item.category, t)}
                    </div>
                    <div className='text-muted-foreground text-xs'>
                      #{item.id}
                    </div>
                  </TableCell>
                  <TableCell className='max-w-64 break-all'>
                    {item.recipient || '-'}
                  </TableCell>
                  <TableCell>
                    <Badge variant='outline' className={meta.className}>
                      {t(meta.label)}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    {item.sender_account_name ||
                      (item.sender_account_id
                        ? `#${item.sender_account_id}`
                        : '-')}
                  </TableCell>
                  <TableCell>{item.attempts}</TableCell>
                  <TableCell>
                    {item.status === 'retrying' && item.next_attempt_time
                      ? formatTimestampToDate(item.next_attempt_time)
                      : '-'}
                  </TableCell>
                  <TableCell className='max-w-72 whitespace-normal'>
                    <span className='line-clamp-2' title={item.last_error}>
                      {item.last_error || '-'}
                    </span>
                  </TableCell>
                  <TableCell>
                    {formatTimestampToDate(item.created_time)}
                  </TableCell>
                  <TableCell>
                    {item.delivered_time ||
                    item.dead_letter_time ||
                    item.expired_time
                      ? formatTimestampToDate(
                          item.delivered_time ||
                            item.dead_letter_time ||
                            item.expired_time
                        )
                      : '-'}
                  </TableCell>
                  <TableCell className='text-right'>
                    {item.status === 'failed' ? (
                      <Button
                        type='button'
                        size='sm'
                        variant='outline'
                        disabled={retrying}
                        onClick={() => void retry([item.id])}
                      >
                        <RotateCcw className='size-4' />
                        {t('Retry')}
                      </Button>
                    ) : null}
                  </TableCell>
                </TableRow>
              )
            })}
            {!queueQuery.isLoading && items.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={11}
                  className='text-muted-foreground h-28 text-center'
                >
                  {t('No email queue records')}
                </TableCell>
              </TableRow>
            ) : null}
          </TableBody>
        </Table>
      </div>

      <div className='flex items-center justify-between'>
        <span className='text-muted-foreground text-sm'>
          {t('{{count}} records', { count: total })}
        </span>
        <div className='flex gap-2'>
          <Button
            type='button'
            variant='outline'
            size='sm'
            disabled={page <= 1}
            onClick={() => setPage((current) => current - 1)}
          >
            {t('Previous')}
          </Button>
          <Button
            type='button'
            variant='outline'
            size='sm'
            disabled={page >= totalPages}
            onClick={() => setPage((current) => current + 1)}
          >
            {t('Next')}
          </Button>
        </div>
      </div>
    </div>
  )
}

export function EmailQueueRulesSection() {
  const statsQuery = useQuery({
    queryKey: ['email-queue', 'stats'],
    queryFn: fetchStats,
  })

  return (
    <EmailQueueRulesCard
      rules={statsQuery.data?.rules}
      onSaved={async () => {
        await statsQuery.refetch()
      }}
    />
  )
}

export function EmailQueueOverviewStats() {
  const { t } = useTranslation()
  const statsQuery = useQuery({
    queryKey: ['email-queue', 'stats'],
    queryFn: fetchStats,
    refetchInterval: 30_000,
  })
  const stats = statsQuery.data?.queue
  return (
    <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-4 xl:grid-cols-7'>
      <QueueStat label={t('Queued')} value={stats?.queued ?? 0} />
      <QueueStat label={t('Sending')} value={stats?.sending ?? 0} />
      <QueueStat label={t('Waiting for retry')} value={stats?.retrying ?? 0} />
      <QueueStat
        label={t('Waiting for receipt')}
        value={stats?.awaiting_receipt ?? 0}
      />
      <QueueStat
        label={t('SMTP accepted without receipt in 24 hours')}
        value={stats?.accepted_untracked_24h ?? 0}
      />
      <QueueStat label={t('Final failures')} value={stats?.failed ?? 0} />
      <QueueStat
        label={t('Final delivered in 24 hours')}
        value={stats?.final_delivered_24h ?? 0}
      />
    </div>
  )
}

function QueueStat(props: { label: string; value: number | string }) {
  return (
    <Card data-card-hover='false'>
      <CardContent className='py-4'>
        <div className='text-muted-foreground text-xs'>{props.label}</div>
        <div className='mt-1 text-2xl font-semibold'>{props.value}</div>
      </CardContent>
    </Card>
  )
}

function HealthItem(props: { label: string; value: string }) {
  return (
    <div>
      <div className='text-muted-foreground text-xs'>{props.label}</div>
      <div className='mt-1 text-sm font-medium'>{props.value}</div>
    </div>
  )
}

function formatWaitingTime(
  createdTime: number,
  t: (key: string, options?: Record<string, unknown>) => string
) {
  const minutes = Math.max(0, Math.floor(Date.now() / 1000 - createdTime) / 60)
  if (minutes < 60) {
    return t('{{count}} minutes', { count: Math.floor(minutes) })
  }
  return t('{{count}} hours', { count: Math.floor(minutes / 60) })
}

function queueHealthLabel(
  stats: EmailQueueStats['queue'] | undefined,
  t: (key: string) => string
) {
  if (!stats) return '-'
  if (stats.failed >= 20 || stats.failure_rate_24h >= 0.2) {
    return t('Attention required')
  }
  if (stats.queued + stats.retrying >= 100) {
    return t('Backlog detected')
  }
  return t('Healthy')
}

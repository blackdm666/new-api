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
import { useCanGoBack, useNavigate, useRouter } from '@tanstack/react-router'
import {
  ArrowLeft,
  Archive,
  Copy,
  History,
  Loader2,
  Mail,
  RefreshCw,
  Send,
  Trash2,
  User,
  XCircle,
} from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { CopyButton } from '@/components/copy-button'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { getPaymentMethodName } from '@/features/wallet/lib/billing'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { toIntlLocale } from '@/i18n/languages'
import { formatQuotaWithCurrency } from '@/lib/currency'
import { getUserFacingErrorMessage } from '@/lib/user-facing-error'
import { cn } from '@/lib/utils'

import {
  fetchInvoiceConfig,
  fetchInvoiceRequest,
  fetchInvoiceUserProfile,
  invoiceQueryKeys,
  purgeInvoiceRequest,
  resendIssuedInvoiceNotification,
  retryInvoiceNotification,
  withdrawInvoiceRequest,
} from '../api'
import { formatInvoiceCopyText } from '../lib/invoice-copy'
import {
  formatInvoiceAmount,
  formatInvoiceTimestamp,
  getInvoiceNotificationChannelLabel,
  getInvoiceNotificationKindLabel,
  INVOICE_STATUS_META,
  isInvoiceRequestExpiring,
} from '../lib/invoice-utils'
import {
  INVOICE_STATUS,
  type InvoiceRequestDetail,
  type InvoiceStatus,
  type InvoiceUserProfile,
} from '../types'
import { InvoiceFilesSection } from './invoice-files-section'
import { InvoiceStatusControls } from './invoice-status-controls'

type InvoiceDetailPageProps = {
  invoiceId: number
  admin: boolean
  /** Scroll to the files section once the detail has loaded. */
  scrollToFiles?: boolean
}

export function InvoiceDetailPage(props: InvoiceDetailPageProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const navigate = useNavigate()
  const router = useRouter()
  const canGoBack = useCanGoBack()
  const queryClient = useQueryClient()
  const admin = props.admin
  const validId = props.invoiceId > 0
  const [confirmation, setConfirmation] = useState<'withdraw' | 'purge' | null>(
    null
  )
  const [confirmationBusy, setConfirmationBusy] = useState(false)
  const filesRef = useRef<HTMLDivElement>(null)
  const scrolledToFiles = useRef(false)

  const detailQuery = useQuery({
    queryKey: invoiceQueryKeys.detail(admin, props.invoiceId),
    queryFn: () => fetchInvoiceRequest(props.invoiceId, admin),
    enabled: validId,
    retry: false,
    meta: { errorToast: false },
  })
  const detail = detailQuery.data
  const adminCanUpload =
    admin && detail?.invoice.status === INVOICE_STATUS.PENDING
  const configQuery = useQuery({
    queryKey: invoiceQueryKeys.config(),
    queryFn: fetchInvoiceConfig,
    enabled: adminCanUpload,
    retry: false,
    meta: { errorToast: false },
  })

  useEffect(() => {
    if (!props.scrollToFiles || !detail || scrolledToFiles.current) return
    scrolledToFiles.current = true
    filesRef.current?.scrollIntoView({ block: 'start' })
  }, [detail, props.scrollToFiles])

  const goBack = () => {
    // History back keeps the list filters and page stored in the URL.
    if (canGoBack) {
      router.history.back()
      return
    }
    void navigate({ to: admin ? '/admin-invoices' : '/invoices' })
  }

  const refresh = async () => {
    void queryClient.invalidateQueries({ queryKey: invoiceQueryKeys.lists() })
    void queryClient.invalidateQueries({ queryKey: invoiceQueryKeys.config() })
    const result = await detailQuery.refetch()
    if (result.isError) toast.error(getUserFacingErrorMessage(result.error))
  }

  const confirmFinalAction = async () => {
    if (!confirmation || confirmationBusy) return
    setConfirmationBusy(true)
    try {
      if (confirmation === 'withdraw') {
        await withdrawInvoiceRequest(props.invoiceId)
        toast.success(t('Invoice application withdrawn'))
        setConfirmation(null)
        await refresh()
      } else {
        await purgeInvoiceRequest(props.invoiceId)
        toast.success(t('Invoice application deleted'))
        setConfirmation(null)
        queryClient.removeQueries({
          queryKey: invoiceQueryKeys.detail(admin, props.invoiceId),
        })
        void queryClient.invalidateQueries({
          queryKey: invoiceQueryKeys.lists(),
        })
        goBack()
      }
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setConfirmationBusy(false)
    }
  }

  if (!validId) {
    return (
      <div className='h-full overflow-y-auto px-4 py-6 sm:px-8'>
        <EmptyState
          title={t('Invoice not found')}
          action={
            <Button variant='outline' onClick={goBack}>
              {t('Back')}
            </Button>
          }
        />
      </div>
    )
  }
  if (!detail && detailQuery.isError) {
    return (
      <div className='h-full overflow-y-auto px-4 py-6 sm:px-8'>
        <ErrorState
          title={t('Failed to load the invoice application')}
          description={getUserFacingErrorMessage(detailQuery.error)}
          onRetry={() => void detailQuery.refetch()}
          action={
            <Button variant='ghost' size='sm' onClick={goBack}>
              {t('Back')}
            </Button>
          }
        />
      </div>
    )
  }
  if (!detail) {
    return (
      <div className='h-full space-y-4 overflow-y-auto px-4 py-6 sm:px-8'>
        <Skeleton className='h-10 w-48' />
        <Skeleton className='h-72 w-full' />
      </div>
    )
  }

  const invoice = detail.invoice
  const files = detail.files
  const meta = INVOICE_STATUS_META[invoice.status]
  const expiring = isInvoiceRequestExpiring(invoice)
  const loading = detailQuery.isFetching
  const config = configQuery.data
  const taxFee = (Number(invoice.tax_fee_cents || 0) / 100).toFixed(2)
  return (
    <div className='h-full overflow-y-auto px-4 py-6 sm:px-8'>
      <div className='mx-auto w-full max-w-[1200px] space-y-5'>
        <div className='flex flex-wrap items-center justify-between gap-3'>
          <Button variant='ghost' onClick={goBack}>
            <ArrowLeft className='size-4' />
            {t('Back')}
          </Button>
          <div className='flex flex-wrap items-center gap-2'>
            {!admin && invoice.status === INVOICE_STATUS.PENDING && (
              <Button
                variant='outline'
                onClick={() => setConfirmation('withdraw')}
              >
                <XCircle className='size-4' />
                {t('Withdraw application')}
              </Button>
            )}
            {admin &&
              (invoice.status === INVOICE_STATUS.REJECTED ||
                invoice.status === INVOICE_STATUS.WITHDRAWN ||
                invoice.status === INVOICE_STATUS.EXPIRED) && (
                <Button
                  variant='destructive'
                  onClick={() => setConfirmation('purge')}
                >
                  <Trash2 className='size-4' />
                  {t('Delete application')}
                </Button>
              )}
            <Button
              variant='outline'
              onClick={() => void refresh()}
              disabled={loading}
            >
              <RefreshCw className={cn('size-4', loading && 'animate-spin')} />
              {t('Refresh')}
            </Button>
          </div>
        </div>

        <header className='flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between'>
          <div>
            <div className='flex items-center gap-2'>
              <h1 className='text-xl font-semibold'>
                {t('Invoice application')} #{invoice.id}
              </h1>
              <Badge
                variant='outline'
                className={cn('font-normal', meta.badgeClass)}
              >
                {t(meta.labelKey)}
              </Badge>
              {expiring && (
                <Badge
                  variant='outline'
                  className='border-orange-500/40 bg-orange-500/10 text-orange-600 dark:text-orange-400'
                >
                  {t('Expires soon')}
                </Badge>
              )}
            </div>
            <p className='text-muted-foreground mt-1 text-sm'>
              {t('Submitted at')}{' '}
              {formatInvoiceTimestamp(invoice.created_time, locale)}
            </p>
            {invoice.issued_time > 0 && (
              <p className='text-muted-foreground mt-1 text-sm'>
                {t('Issued at')}{' '}
                {formatInvoiceTimestamp(invoice.issued_time, locale)}
              </p>
            )}
            {invoice.status === INVOICE_STATUS.PENDING &&
              invoice.expires_at > 0 && (
                <p className='text-muted-foreground mt-1 text-xs'>
                  {t('Expires at')}{' '}
                  {formatInvoiceTimestamp(invoice.expires_at, locale)}
                </p>
              )}
          </div>
          {admin && (
            <div className='flex flex-wrap items-start gap-2'>
              <UserProfileSheet
                invoiceId={invoice.id}
                username={invoice.username}
                userId={invoice.user_id}
              />
              <InvoiceStatusControls
                key={`${invoice.id}-${invoice.status}`}
                invoice={invoice}
                fileCount={files.length}
                onUpdated={refresh}
              />
            </div>
          )}
        </header>

        {invoice.status === INVOICE_STATUS.REJECTED &&
          invoice.rejection_reason && (
            <div className='border-destructive/30 bg-destructive/5 rounded-xl border px-4 py-3'>
              <div className='text-sm font-medium'>{t('Rejection reason')}</div>
              <p className='text-muted-foreground mt-1 text-sm'>
                {invoice.rejection_reason}
              </p>
            </div>
          )}

        {invoice.redacted_time > 0 && (
          <div className='bg-muted/40 flex items-start gap-3 rounded-xl border px-4 py-3'>
            <Archive
              className='text-muted-foreground mt-0.5 size-4 shrink-0'
              aria-hidden='true'
            />
            <div>
              <div className='text-sm font-medium'>
                {t('Invoice details archived')}
              </div>
              <p className='text-muted-foreground mt-1 text-sm'>
                {t(
                  'Personal billing details and invoice files were removed under the configured retention policy. Financial totals, status history, and order claims were retained.'
                )}
              </p>
            </div>
          </div>
        )}

        <InvoiceInformation detail={detail} admin={admin} locale={locale} />
        <div ref={filesRef} id='invoice-files' className='scroll-mt-6'>
          <InvoiceFilesSection
            invoiceId={invoice.id}
            files={files}
            admin={admin}
            invoiceStatus={invoice.status}
            redacted={invoice.redacted_time > 0}
            locale={locale}
            uploadLimits={
              config
                ? {
                    allowedExts: config.file_allowed_exts,
                    maxCount: config.file_max_count,
                    maxSize: config.file_max_size,
                  }
                : undefined
            }
            onChanged={refresh}
          />
        </div>
        {admin && (
          <InvoiceNotifications
            detail={detail}
            locale={locale}
            onChanged={refresh}
          />
        )}
        <InvoiceHistory detail={detail} admin={admin} locale={locale} />
      </div>
      <ConfirmDialog
        open={confirmation !== null}
        onOpenChange={(open) => {
          if (!open && !confirmationBusy) setConfirmation(null)
        }}
        title={
          confirmation === 'purge'
            ? t('Delete this invoice application permanently?')
            : t('Withdraw this invoice application?')
        }
        desc={
          confirmation === 'purge' ? (
            t(
              'The application record will be deleted and any remaining files will be queued for cleanup. This cannot be undone.'
            )
          ) : (
            <div className='space-y-2'>
              <p>
                {t(
                  'The selected orders will be released and can be used in a new invoice application.'
                )}
              </p>
              {Number(invoice.tax_fee_cents || 0) > 0 && (
                <p>
                  {t(
                    'The tax fee of ¥{{amount}} will be refunded to your balance.',
                    { amount: taxFee }
                  )}
                </p>
              )}
            </div>
          )
        }
        confirmText={
          <>
            {confirmationBusy && <Loader2 className='size-4 animate-spin' />}
            {confirmation === 'purge'
              ? t('Confirm deletion')
              : t('Confirm withdrawal')}
          </>
        }
        destructive={confirmation === 'purge'}
        isLoading={confirmationBusy}
        handleConfirm={() => void confirmFinalAction()}
      />
    </div>
  )
}

type InvoiceInformationProps = {
  detail: InvoiceRequestDetail
  admin: boolean
  locale?: string
}

function InvoiceInformation(props: InvoiceInformationProps) {
  const { t } = useTranslation()
  const { copyToClipboard } = useCopyToClipboard()
  const invoice = props.detail.invoice
  const orders = props.detail.orders
  const canCopy = props.admin && invoice.redacted_time === 0
  const paymentLabel = (method: string) => getPaymentMethodName(method, t)
  const copyAll = () => {
    const text = formatInvoiceCopyText(invoice, {
      companyName: t('Invoice title'),
      taxNumber: t('Tax number'),
      bankName: t('Bank name'),
      bankAccount: t('Bank account'),
      companyAddress: t('Company address'),
      companyPhone: t('Company phone'),
      remark: t('Remark'),
      totalAmount: t('Total amount'),
    })
    void copyToClipboard(text)
  }
  const fields = [
    [t('Invoice title'), invoice.company_name],
    [t('Tax number'), invoice.tax_number],
    [t('Bank name'), invoice.bank_name],
    [t('Bank account'), invoice.bank_account],
    [t('Company address'), invoice.company_address],
    [t('Company phone'), invoice.company_phone],
  ].filter(([, value]) => value)
  const taxFeeRefundedTime = Number(invoice.tax_fee_refunded_time || 0)
  return (
    <section className='bg-card rounded-2xl border p-4 sm:p-5'>
      <div className='flex items-center justify-between gap-3'>
        <h2 className='font-semibold'>{t('Invoice information')}</h2>
        {canCopy && (
          <Button variant='outline' onClick={copyAll}>
            <Copy className='size-4' />
            {t('Copy invoice information')}
          </Button>
        )}
      </div>
      <dl className='mt-4 grid gap-4 sm:grid-cols-2'>
        {fields.map(([label, value]) => (
          <div key={label}>
            <dt className='text-muted-foreground text-xs'>{label}</dt>
            <dd className='mt-1 flex items-start gap-1 text-sm'>
              <span className='min-w-0 flex-1 break-all'>{value}</span>
              {canCopy && (
                <CopyButton
                  value={value}
                  className='-my-1 size-7'
                  iconClassName='size-3.5'
                  aria-label={t('Copy {{field}}', { field: label })}
                />
              )}
            </dd>
          </div>
        ))}
        <div>
          <dt className='text-muted-foreground text-xs'>{t('Total amount')}</dt>
          <dd className='text-primary mt-1 text-xl font-semibold'>
            {formatInvoiceAmount(invoice.total_money)}
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('Invoice tax rate')}
          </dt>
          <dd className='mt-1 text-sm font-semibold'>
            {(Number(invoice.tax_rate_basis_points || 0) / 100).toFixed(2)}%
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground text-xs'>{t('Tax fee paid')}</dt>
          <dd className='mt-1 text-sm font-semibold'>
            ¥{(Number(invoice.tax_fee_cents || 0) / 100).toFixed(2)}
          </dd>
          {taxFeeRefundedTime > 0 && (
            <dd className='mt-1 text-xs text-emerald-600 dark:text-emerald-400'>
              {t('Tax fee refunded: ¥{{amount}} ({{time}})', {
                amount: (Number(invoice.tax_fee_cents || 0) / 100).toFixed(2),
                time: formatInvoiceTimestamp(taxFeeRefundedTime, props.locale),
              })}
            </dd>
          )}
        </div>
        {invoice.remark && (
          <div className='sm:col-span-2'>
            <dt className='text-muted-foreground text-xs'>{t('Remark')}</dt>
            <dd className='mt-1 text-sm whitespace-pre-wrap'>
              {invoice.remark}
            </dd>
          </div>
        )}
      </dl>
      <h3 className='mt-6 mb-2 text-sm font-semibold'>
        {t('Included top-up orders')}
      </h3>
      <div className='overflow-x-auto rounded-lg border'>
        <table className='w-full text-sm'>
          <thead className='bg-muted/40 text-muted-foreground text-xs'>
            <tr>
              <th className='px-3 py-2 text-left'>{t('Order number')}</th>
              <th className='px-3 py-2 text-left'>{t('Payment')}</th>
              <th className='px-3 py-2 text-right'>{t('Amount')}</th>
              <th className='px-3 py-2 text-left'>{t('Completed at')}</th>
            </tr>
          </thead>
          <tbody>
            {orders.length === 0 ? (
              <tr className='border-t'>
                <td
                  colSpan={4}
                  className='text-muted-foreground px-3 py-8 text-center text-sm'
                >
                  {invoice.redacted_time > 0
                    ? t('Order details removed by retention policy')
                    : t('No eligible paid orders')}
                </td>
              </tr>
            ) : (
              orders.map((order) => (
                <tr key={order.id} className='border-t'>
                  <td className='px-3 py-2 font-mono text-xs'>
                    {order.trade_no || `#${order.id}`}
                  </td>
                  <td className='px-3 py-2'>
                    {order.payment_method
                      ? paymentLabel(order.payment_method)
                      : '—'}
                  </td>
                  <td className='px-3 py-2 text-right font-semibold'>
                    ¥{Number(order.money || 0).toFixed(2)}
                  </td>
                  <td className='text-muted-foreground px-3 py-2 text-xs'>
                    {formatInvoiceTimestamp(
                      order.complete_time || order.create_time,
                      props.locale
                    )}
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </section>
  )
}

type InvoiceNotificationsProps = {
  detail: InvoiceRequestDetail
  locale?: string
  onChanged: () => Promise<void>
}

function InvoiceNotifications(props: InvoiceNotificationsProps) {
  const { t } = useTranslation()
  const [busyId, setBusyId] = useState<number | 'resend' | null>(null)
  const notifications = props.detail.notifications || []
  const invoice = props.detail.invoice

  const retry = async (id: number) => {
    if (busyId !== null) return
    setBusyId(id)
    try {
      await retryInvoiceNotification(id)
      toast.success(t('Notification queued'))
      await props.onChanged()
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setBusyId(null)
    }
  }

  const resend = async () => {
    if (busyId !== null) return
    setBusyId('resend')
    try {
      await resendIssuedInvoiceNotification(invoice.id)
      toast.success(t('Invoice notification queued'))
      await props.onChanged()
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setBusyId(null)
    }
  }

  return (
    <section className='bg-card rounded-2xl border p-4 sm:p-5'>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <div className='flex items-center gap-2'>
          <Mail className='text-primary size-4' aria-hidden='true' />
          <h2 className='font-semibold'>{t('Invoice notifications')}</h2>
        </div>
        {invoice.status === INVOICE_STATUS.ISSUED && (
          <Button
            variant='outline'
            size='sm'
            onClick={() => void resend()}
            disabled={busyId !== null}
          >
            {busyId === 'resend' ? (
              <Loader2 className='size-4 animate-spin' />
            ) : (
              <Send className='size-4' />
            )}
            {t('Resend issued invoice email')}
          </Button>
        )}
      </div>
      {notifications.length === 0 ? (
        <div className='text-muted-foreground mt-4 rounded-lg border border-dashed px-4 py-8 text-center text-sm'>
          {t('No invoice notifications')}
        </div>
      ) : (
        <div className='mt-4 overflow-x-auto rounded-lg border'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Notification channel')}</TableHead>
                <TableHead>{t('Type')}</TableHead>
                <TableHead>{t('Recipient')}</TableHead>
                <TableHead>{t('Status')}</TableHead>
                <TableHead>{t('Last error')}</TableHead>
                <TableHead>{t('Updated')}</TableHead>
                <TableHead className='text-right'>{t('Operation')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {notifications.map((notification) => {
                const delivered = notification.delivered_time > 0
                const failed = !delivered && notification.last_error
                let statusLabel = t('Queued')
                if (delivered) {
                  statusLabel = t('Delivered')
                } else if (failed) {
                  statusLabel = t('Failed')
                }
                return (
                  <TableRow key={notification.id}>
                    <TableCell className='whitespace-nowrap'>
                      {getInvoiceNotificationChannelLabel(
                        notification.channel,
                        t
                      )}
                    </TableCell>
                    <TableCell className='whitespace-nowrap'>
                      {getInvoiceNotificationKindLabel(notification.kind, t)}
                    </TableCell>
                    <TableCell className='max-w-[240px] truncate text-xs'>
                      {notification.recipient || '—'}
                    </TableCell>
                    <TableCell>
                      <Badge
                        variant='outline'
                        className={cn(
                          'font-normal',
                          delivered &&
                            'border-emerald-500/40 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400',
                          failed &&
                            'border-rose-500/40 bg-rose-500/10 text-rose-600 dark:text-rose-400'
                        )}
                      >
                        {statusLabel}
                      </Badge>
                    </TableCell>
                    <TableCell className='max-w-[320px] truncate text-xs'>
                      {notification.last_error || '—'}
                    </TableCell>
                    <TableCell className='text-muted-foreground text-xs whitespace-nowrap'>
                      {formatInvoiceTimestamp(
                        notification.delivered_time ||
                          notification.updated_time ||
                          notification.created_time,
                        props.locale
                      )}
                    </TableCell>
                    <TableCell className='text-right'>
                      {!delivered && (
                        <Button
                          variant='ghost'
                          size='sm'
                          disabled={busyId !== null}
                          onClick={() => void retry(notification.id)}
                        >
                          {busyId === notification.id && (
                            <Loader2 className='size-4 animate-spin' />
                          )}
                          {t('Retry')}
                        </Button>
                      )}
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </div>
      )}
    </section>
  )
}

type InvoiceHistoryProps = {
  detail: InvoiceRequestDetail
  admin: boolean
  locale?: string
}

function InvoiceHistory(props: InvoiceHistoryProps) {
  const { t } = useTranslation()
  if (!props.detail.events?.length) return null
  return (
    <section className='bg-card rounded-2xl border p-4 sm:p-5'>
      <div className='flex items-center gap-2'>
        <History className='text-primary size-4' aria-hidden='true' />
        <h2 className='font-semibold'>{t('Application history')}</h2>
      </div>
      <ol className='mt-4 space-y-3'>
        {props.detail.events.map((event) => {
          let operator = ''
          if (props.admin && event.operator_name) {
            operator = event.operator_id
              ? t('Operator: {{name}} (UID {{id}})', {
                  name: event.operator_name,
                  id: event.operator_id,
                })
              : t('Operator: {{name}}', { name: event.operator_name })
          } else if (props.admin && event.operator_id) {
            operator = t('Operator UID: {{id}}', { id: event.operator_id })
          }
          return (
            <li key={event.id} className='rounded-lg border px-3 py-2 text-sm'>
              <div className='flex flex-wrap items-center justify-between gap-2'>
                <span className='font-medium'>
                  {event.from_status === 0
                    ? t('Invoice application submitted')
                    : t('Status changed from {{from}} to {{to}}', {
                        from: t(
                          INVOICE_STATUS_META[
                            event.from_status as InvoiceStatus
                          ].labelKey
                        ),
                        to: t(INVOICE_STATUS_META[event.to_status].labelKey),
                      })}
                </span>
                <span className='text-muted-foreground text-xs'>
                  {formatInvoiceTimestamp(event.created_time, props.locale)}
                </span>
              </div>
              {event.reason && (
                <p className='text-muted-foreground mt-1'>{event.reason}</p>
              )}
              {operator && (
                <p className='text-muted-foreground mt-1 text-xs'>{operator}</p>
              )}
            </li>
          )
        })}
      </ol>
    </section>
  )
}

function UserProfileSheet({
  invoiceId,
  username,
  userId,
}: {
  invoiceId: number
  username?: string
  userId: number
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [loading, setLoading] = useState(false)
  const [profile, setProfile] = useState<InvoiceUserProfile | null>(null)
  const load = async () => {
    setLoading(true)
    try {
      setProfile(await fetchInvoiceUserProfile(invoiceId))
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setLoading(false)
    }
  }
  useEffect(() => {
    if (open && !profile && !loading) void load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])
  let profileContent: React.ReactNode
  if (loading && !profile) {
    profileContent = <Skeleton className='h-48 w-full' />
  } else if (profile) {
    profileContent = (
      <div className='space-y-6'>
        <dl className='grid gap-4 rounded-xl border p-4 sm:grid-cols-2'>
          <ProfileRow label={t('User')}>
            {profile.username} (UID {profile.user_id})
          </ProfileRow>
          <ProfileRow label={t('Display name')}>
            {profile.display_name || '—'}
          </ProfileRow>
          <ProfileRow label={t('Email')}>{profile.email || '—'}</ProfileRow>
          <ProfileRow label={t('Group')}>
            {profile.group || 'default'}
          </ProfileRow>
          <ProfileRow label={t('Current balance')}>
            {formatQuotaWithCurrency(profile.quota || 0)}
          </ProfileRow>
          <ProfileRow label={t('Total used')}>
            {formatQuotaWithCurrency(profile.used_quota || 0)}
          </ProfileRow>
          <ProfileRow label={t('Request count')}>
            {Number(profile.request_count || 0).toLocaleString()}
          </ProfileRow>
          <ProfileRow label={t('Registered at')}>
            {formatInvoiceTimestamp(profile.created_time)}
          </ProfileRow>
        </dl>
        <ProfileUsageTables profile={profile} />
      </div>
    )
  } else {
    profileContent = (
      <p className='text-muted-foreground py-12 text-center text-sm'>
        {t('No user profile data')}
      </p>
    )
  }
  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger render={<Button variant='outline' />}>
        <User className='size-4' />
        {t('User profile')}
      </SheetTrigger>
      <SheetContent
        className='w-full overflow-y-auto sm:max-w-3xl'
        closeLabel={t('Close')}
      >
        <SheetHeader>
          <SheetTitle>{t('User profile')}</SheetTitle>
          <SheetDescription>
            {username || '—'} · UID {userId}
          </SheetDescription>
        </SheetHeader>
        <div className='px-4 pb-4'>{profileContent}</div>
      </SheetContent>
    </Sheet>
  )
}

function ProfileUsageTables({ profile }: { profile: InvoiceUserProfile }) {
  const { t } = useTranslation()
  const recentLogs = profile.recent_logs || []
  const modelUsage = profile.model_usage || []
  return (
    <div className='space-y-6 pb-6'>
      <ProfileTableSection title={t('Recent API calls')}>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className='whitespace-nowrap'>{t('Time')}</TableHead>
              <TableHead>{t('Model')}</TableHead>
              <TableHead className='text-right'>{t('Fee')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {recentLogs.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={3}
                  className='text-muted-foreground h-24 text-center'
                >
                  {t('No recent API calls')}
                </TableCell>
              </TableRow>
            ) : (
              recentLogs.map((log) => (
                <TableRow key={`${log.created_at}-${log.id}`}>
                  <TableCell className='whitespace-nowrap'>
                    {formatInvoiceTimestamp(log.created_at)}
                  </TableCell>
                  <TableCell className='max-w-48 truncate font-medium'>
                    {log.model_name || '—'}
                  </TableCell>
                  <TableCell className='text-right whitespace-nowrap'>
                    {formatQuotaWithCurrency(log.quota || 0)}
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </ProfileTableSection>

      <ProfileTableSection title={t('Top models (last 30 days)')}>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Model')}</TableHead>
              <TableHead className='text-right'>{t('Calls')}</TableHead>
              <TableHead className='text-right'>{t('Fee')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {modelUsage.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={3}
                  className='text-muted-foreground h-24 text-center'
                >
                  {t('No model usage data')}
                </TableCell>
              </TableRow>
            ) : (
              modelUsage.map((usage) => (
                <TableRow key={usage.model_name}>
                  <TableCell className='font-medium'>
                    {usage.model_name || '—'}
                  </TableCell>
                  <TableCell className='text-right tabular-nums'>
                    {Number(usage.count || 0).toLocaleString()}
                  </TableCell>
                  <TableCell className='text-right whitespace-nowrap'>
                    {formatQuotaWithCurrency(usage.quota || 0)}
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </ProfileTableSection>
    </div>
  )
}

function ProfileTableSection({
  title,
  children,
}: {
  title: string
  children: React.ReactNode
}) {
  return (
    <section className='space-y-2'>
      <h3 className='text-sm font-semibold'>{title}</h3>
      <div className='overflow-x-auto rounded-xl border'>{children}</div>
    </section>
  )
}

function ProfileRow({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div>
      <dt className='text-muted-foreground text-xs'>{label}</dt>
      <dd className='mt-1 text-sm'>{children}</dd>
    </div>
  )
}

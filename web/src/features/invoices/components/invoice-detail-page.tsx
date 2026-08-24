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
import { useNavigate } from '@tanstack/react-router'
import {
  ArrowLeft,
  Archive,
  Copy,
  Download,
  Eye,
  FileText,
  History,
  Loader2,
  Mail,
  RefreshCw,
  Send,
  Trash2,
  Upload,
  User,
  XCircle,
} from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
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
import { Textarea } from '@/components/ui/textarea'
import { formatQuotaWithCurrency } from '@/lib/currency'
import { getUserFacingErrorMessage } from '@/lib/user-facing-error'
import { cn } from '@/lib/utils'

import {
  deleteInvoiceFile,
  downloadInvoiceFile,
  fetchInvoiceRequest,
  fetchInvoiceUserProfile,
  purgeInvoiceRequest,
  resendIssuedInvoiceNotification,
  retryInvoiceNotification,
  updateInvoiceStatus,
  uploadInvoiceFile,
  withdrawInvoiceRequest,
} from '../api'
import { formatInvoiceCopyText } from '../lib/invoice-copy'
import {
  formatInvoiceTimestamp,
  INVOICE_STATUS_META,
  isInvoiceRequestExpiring,
  isPreviewableInvoiceFile,
} from '../lib/invoice-utils'
import {
  INVOICE_STATUS,
  type InvoiceFile,
  type InvoiceRequestDetail,
  type InvoiceStatus,
  type InvoiceUserProfile,
} from '../types'

export function InvoiceDetailPage({
  invoiceId,
  admin,
}: {
  invoiceId: number
  admin: boolean
}) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [detail, setDetail] = useState<InvoiceRequestDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [status, setStatus] = useState<InvoiceStatus | null>(null)
  const [saving, setSaving] = useState(false)
  const [rejectionReason, setRejectionReason] = useState('')
  const [confirmation, setConfirmation] = useState<'withdraw' | 'purge' | null>(
    null
  )
  const [confirmationBusy, setConfirmationBusy] = useState(false)

  const refresh = useCallback(async () => {
    if (!invoiceId) return
    setLoading(true)
    try {
      const result = await fetchInvoiceRequest(invoiceId, admin)
      setDetail(result)
      setStatus(result.invoice.status)
      setRejectionReason(result.invoice.rejection_reason || '')
    } catch (error) {
      setDetail(null)
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setLoading(false)
    }
  }, [admin, invoiceId])

  useEffect(() => {
    const timeout = window.setTimeout(() => {
      void refresh()
    }, 0)
    return () => window.clearTimeout(timeout)
  }, [refresh])

  const saveStatus = async () => {
    if (!status || !detail || saving) return
    if (status === detail.invoice.status) {
      toast.info(t('No changes to save'))
      return
    }
    if (status === INVOICE_STATUS.REJECTED && !rejectionReason.trim()) {
      toast.error(t('Rejection reason is required'))
      return
    }
    setSaving(true)
    try {
      await updateInvoiceStatus(invoiceId, status, rejectionReason.trim())
      toast.success(t('Invoice status updated'))
      await refresh()
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setSaving(false)
    }
  }

  const confirmFinalAction = async () => {
    if (!confirmation || confirmationBusy) return
    setConfirmationBusy(true)
    try {
      if (confirmation === 'withdraw') {
        await withdrawInvoiceRequest(invoiceId)
        toast.success(t('Invoice application withdrawn'))
        setConfirmation(null)
        await refresh()
      } else {
        await purgeInvoiceRequest(invoiceId)
        toast.success(t('Invoice application deleted'))
        setConfirmation(null)
        navigate({ to: '/admin-invoices' })
      }
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setConfirmationBusy(false)
    }
  }

  if (loading && !detail) {
    return (
      <div className='h-full overflow-y-auto px-4 py-6 sm:px-8'>
        <Skeleton className='h-10 w-48' />
        <Skeleton className='h-72 w-full' />
      </div>
    )
  }
  if (!detail) {
    return (
      <div className='px-4 py-16 text-center sm:px-8'>
        <p className='text-muted-foreground'>{t('Invoice not found')}</p>
        <Button
          className='mt-4'
          variant='outline'
          onClick={() =>
            navigate({ to: admin ? '/admin-invoices' : '/invoices' })
          }
        >
          {t('Back')}
        </Button>
      </div>
    )
  }

  const { invoice, files } = detail
  const meta = INVOICE_STATUS_META[invoice.status]
  const expiring = isInvoiceRequestExpiring(invoice)
  return (
    <div className='h-full overflow-y-auto px-4 py-6 sm:px-8'>
      <div className='mx-auto w-full max-w-[1200px] space-y-5'>
        <div className='flex flex-wrap items-center justify-between gap-3'>
          <Button
            variant='ghost'
            onClick={() =>
              navigate({ to: admin ? '/admin-invoices' : '/invoices' })
            }
          >
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
            <Button variant='outline' onClick={refresh} disabled={loading}>
              <RefreshCw className={cn('size-4', loading && 'animate-spin')} />
              {t('Refresh')}
            </Button>
          </div>
        </div>

        <header className='flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between'>
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
              {t('Submitted at')} {formatInvoiceTimestamp(invoice.created_time)}
            </p>
            {invoice.status === INVOICE_STATUS.PENDING &&
              invoice.expires_at > 0 && (
                <p className='text-muted-foreground mt-1 text-xs'>
                  {t('Expires at')} {formatInvoiceTimestamp(invoice.expires_at)}
                </p>
              )}
          </div>
          {admin && (
            <div className='space-y-2'>
              <div className='flex flex-wrap items-center gap-2'>
                <UserProfileSheet
                  invoiceId={invoice.id}
                  username={invoice.username}
                  userId={invoice.user_id}
                />
                {invoice.status === INVOICE_STATUS.PENDING ? (
                  <>
                    <Label
                      htmlFor='invoice-application-status'
                      className='whitespace-nowrap'
                    >
                      {t('Application status')}
                    </Label>
                    <Select
                      value={status === null ? null : String(status)}
                      onValueChange={(value) => {
                        if (value !== null) {
                          setStatus(Number(value) as InvoiceStatus)
                        }
                      }}
                    >
                      <SelectTrigger
                        id='invoice-application-status'
                        className='w-36'
                      >
                        <SelectValue>
                          {status === null
                            ? t('Application status')
                            : t(INVOICE_STATUS_META[status].labelKey)}
                        </SelectValue>
                      </SelectTrigger>
                      <SelectContent
                        side='bottom'
                        align='start'
                        sideOffset={6}
                        alignItemWithTrigger={false}
                      >
                        <SelectItem
                          value={String(INVOICE_STATUS.PENDING)}
                          disabled
                        >
                          {t(
                            INVOICE_STATUS_META[INVOICE_STATUS.PENDING].labelKey
                          )}
                        </SelectItem>
                        <SelectItem value={String(INVOICE_STATUS.ISSUED)}>
                          {t(
                            INVOICE_STATUS_META[INVOICE_STATUS.ISSUED].labelKey
                          )}
                        </SelectItem>
                        <SelectItem value={String(INVOICE_STATUS.REJECTED)}>
                          {t(
                            INVOICE_STATUS_META[INVOICE_STATUS.REJECTED]
                              .labelKey
                          )}
                        </SelectItem>
                      </SelectContent>
                    </Select>
                    <Button
                      onClick={saveStatus}
                      disabled={
                        saving ||
                        status === null ||
                        status === invoice.status ||
                        (status === INVOICE_STATUS.REJECTED &&
                          !rejectionReason.trim())
                      }
                    >
                      {saving && <Loader2 className='size-4 animate-spin' />}
                      {t('Update status')}
                    </Button>
                  </>
                ) : (
                  <p className='text-muted-foreground text-sm'>
                    {t('Final invoice statuses cannot be changed')}
                  </p>
                )}
              </div>
              {invoice.status === INVOICE_STATUS.PENDING &&
                status === INVOICE_STATUS.REJECTED && (
                  <div className='space-y-1.5 sm:ml-auto sm:w-[420px]'>
                    <Label htmlFor='invoice-rejection-reason'>
                      {t('Rejection reason')}
                    </Label>
                    <Textarea
                      id='invoice-rejection-reason'
                      value={rejectionReason}
                      onChange={(event) =>
                        setRejectionReason(event.target.value)
                      }
                      maxLength={500}
                      rows={3}
                      aria-required='true'
                    />
                  </div>
                )}
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

        <InvoiceInformation detail={detail} admin={admin} />
        <InvoiceFiles
          invoiceId={invoice.id}
          files={files}
          admin={admin}
          invoiceStatus={invoice.status}
          redacted={invoice.redacted_time > 0}
          onChanged={refresh}
        />
        {admin && <InvoiceNotifications detail={detail} onChanged={refresh} />}
        <InvoiceHistory detail={detail} admin={admin} />
      </div>
      <AlertDialog
        open={confirmation !== null}
        onOpenChange={(open) => !open && setConfirmation(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {confirmation === 'withdraw'
                ? t('Withdraw this invoice application?')
                : t('Delete this invoice application permanently?')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {confirmation === 'withdraw'
                ? t(
                    'The selected orders will be released and can be used in a new invoice application.'
                  )
                : t(
                    'The application record will be deleted and any remaining files will be queued for cleanup. This cannot be undone.'
                  )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={confirmationBusy}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              variant={confirmation === 'purge' ? 'destructive' : 'default'}
              disabled={confirmationBusy}
              onClick={() => void confirmFinalAction()}
            >
              {confirmationBusy && <Loader2 className='size-4 animate-spin' />}
              {confirmation === 'withdraw'
                ? t('Confirm withdrawal')
                : t('Confirm deletion')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

function InvoiceInformation({
  detail,
  admin,
}: {
  detail: InvoiceRequestDetail
  admin: boolean
}) {
  const { t } = useTranslation()
  const { invoice, orders } = detail
  const copy = async () => {
    const text = formatInvoiceCopyText(invoice, orders, {
      companyName: t('Company name'),
      taxNumber: t('Tax number'),
      bankName: t('Bank name'),
      bankAccount: t('Bank account'),
      companyAddress: t('Company address'),
      companyPhone: t('Company phone'),
      totalAmount: t('Total amount'),
      includedOrders: t('Included top-up orders'),
      tradeNumber: t('Order number'),
      payment: t('Payment'),
      amount: t('Amount'),
    })
    try {
      await navigator.clipboard.writeText(text)
      toast.success(t('Copied to clipboard'))
    } catch {
      toast.error(t('Failed to copy to clipboard'))
    }
  }
  const fields = [
    [t('Company name'), invoice.company_name],
    [t('Tax number'), invoice.tax_number],
    [t('Bank name'), invoice.bank_name],
    [t('Bank account'), invoice.bank_account],
    [t('Company address'), invoice.company_address],
    [t('Company phone'), invoice.company_phone],
  ].filter(([, value]) => value)
  return (
    <section className='bg-card rounded-2xl border p-4 sm:p-5'>
      <div className='flex items-center justify-between gap-3'>
        <h2 className='font-semibold'>{t('Invoice information')}</h2>
        {admin && invoice.redacted_time === 0 && (
          <Button variant='outline' onClick={copy}>
            <Copy className='size-4' />
            {t('Copy invoice information')}
          </Button>
        )}
      </div>
      <dl className='mt-4 grid gap-4 sm:grid-cols-2'>
        {fields.map(([label, value]) => (
          <div key={label}>
            <dt className='text-muted-foreground text-xs'>{label}</dt>
            <dd className='mt-1 text-sm break-all'>{value}</dd>
          </div>
        ))}
        <div>
          <dt className='text-muted-foreground text-xs'>{t('Total amount')}</dt>
          <dd className='text-primary mt-1 text-xl font-semibold'>
            ¥{Number(invoice.total_money || 0).toFixed(2)}
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
                  <td className='px-3 py-2'>{order.payment_method || '—'}</td>
                  <td className='px-3 py-2 text-right font-semibold'>
                    ¥{Number(order.money || 0).toFixed(2)}
                  </td>
                  <td className='text-muted-foreground px-3 py-2 text-xs'>
                    {formatInvoiceTimestamp(
                      order.complete_time || order.create_time
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

function InvoiceFiles({
  invoiceId,
  files,
  admin,
  invoiceStatus,
  redacted,
  onChanged,
}: {
  invoiceId: number
  files: InvoiceFile[]
  admin: boolean
  invoiceStatus: InvoiceStatus
  redacted: boolean
  onChanged: () => Promise<void>
}) {
  const { t } = useTranslation()
  const inputRef = useRef<HTMLInputElement>(null)
  const [busy, setBusy] = useState(false)

  const upload = async (file?: File) => {
    if (!file || busy) return
    setBusy(true)
    try {
      await uploadInvoiceFile(invoiceId, file)
      toast.success(t('Invoice file uploaded'))
      await onChanged()
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setBusy(false)
      if (inputRef.current) inputRef.current.value = ''
    }
  }
  const remove = async (file: InvoiceFile) => {
    if (busy || !window.confirm(t('Delete this invoice file?'))) return
    setBusy(true)
    try {
      await deleteInvoiceFile(invoiceId, file.id)
      toast.success(t('Invoice file deleted'))
      await onChanged()
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setBusy(false)
    }
  }
  const download = async (file: InvoiceFile, inline = false) => {
    try {
      await downloadInvoiceFile(invoiceId, file, inline)
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    }
  }

  let fileDescription = t('Download the issued electronic invoice here')
  if (invoiceStatus === INVOICE_STATUS.PENDING) {
    if (admin) {
      fileDescription = t(
        'Upload the final PDF or image before marking as issued'
      )
    } else {
      fileDescription = t(
        'The electronic invoice will be available here after it is issued.'
      )
    }
  } else if (invoiceStatus === INVOICE_STATUS.ISSUED) {
    fileDescription = t(
      'Issued invoice files are locked. Use a new application for corrections.'
    )
  }

  return (
    <section className='bg-card rounded-2xl border p-4 sm:p-5'>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <div>
          <h2 className='font-semibold'>{t('Issued invoice files')}</h2>
          <p className='text-muted-foreground mt-1 text-xs'>
            {fileDescription}
          </p>
        </div>
        {admin && !redacted && invoiceStatus === INVOICE_STATUS.PENDING && (
          <>
            <input
              ref={inputRef}
              type='file'
              accept='.jpg,.jpeg,.png,.webp,.pdf'
              className='hidden'
              onChange={(event) => void upload(event.target.files?.[0])}
            />
            <Button
              variant='outline'
              disabled={busy}
              onClick={() => inputRef.current?.click()}
            >
              {busy ? (
                <Loader2 className='size-4 animate-spin' />
              ) : (
                <Upload className='size-4' />
              )}
              {t('Upload invoice file')}
            </Button>
          </>
        )}
      </div>
      {files.length === 0 ? (
        <div className='text-muted-foreground mt-4 rounded-lg border border-dashed px-4 py-10 text-center text-sm'>
          {t('No invoice files')}
        </div>
      ) : (
        <div className='mt-4 space-y-2'>
          {files.map((file) => {
            const previewable = isPreviewableInvoiceFile(file.mime_type)
            return (
              <div
                key={file.id}
                className='flex flex-wrap items-center gap-3 rounded-lg border p-3'
              >
                <FileText className='text-primary h-5 w-5 shrink-0' />
                <div className='min-w-0 flex-1'>
                  <div className='truncate text-sm font-medium'>
                    {file.file_name}
                  </div>
                  <div className='text-muted-foreground text-xs'>
                    {(file.size / 1024).toFixed(1)} KB ·{' '}
                    {formatInvoiceTimestamp(file.created_time)}
                  </div>
                </div>
                {previewable && (
                  <Button
                    variant='ghost'
                    size='sm'
                    onClick={() => void download(file, true)}
                  >
                    <Eye className='size-4' />
                    {t('Preview')}
                  </Button>
                )}
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() => void download(file)}
                >
                  <Download className='size-4' />
                  {t('Download')}
                </Button>
                {admin && invoiceStatus === INVOICE_STATUS.PENDING && (
                  <Button
                    variant='ghost'
                    size='sm'
                    className='text-destructive hover:text-destructive'
                    disabled={busy}
                    aria-label={t('Delete invoice file {{name}}', {
                      name: file.file_name,
                    })}
                    onClick={() => void remove(file)}
                  >
                    <Trash2 className='size-4' />
                  </Button>
                )}
              </div>
            )
          })}
        </div>
      )}
    </section>
  )
}

function InvoiceNotifications({
  detail,
  onChanged,
}: {
  detail: InvoiceRequestDetail
  onChanged: () => Promise<void>
}) {
  const { t } = useTranslation()
  const [busyId, setBusyId] = useState<number | 'resend' | null>(null)
  const notifications = detail.notifications || []
  const invoice = detail.invoice

  const retry = async (id: number) => {
    if (busyId !== null) return
    setBusyId(id)
    try {
      await retryInvoiceNotification(id)
      toast.success(t('Notification queued'))
      await onChanged()
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
      await onChanged()
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
                <TableHead>{t('Type')}</TableHead>
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
                      {notification.kind === 'admin_email'
                        ? t('Admin email')
                        : t('User email')}
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
                          notification.created_time
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

function InvoiceHistory({
  detail,
  admin,
}: {
  detail: InvoiceRequestDetail
  admin: boolean
}) {
  const { t } = useTranslation()
  if (!detail.events?.length) return null
  return (
    <section className='bg-card rounded-2xl border p-4 sm:p-5'>
      <div className='flex items-center gap-2'>
        <History className='text-primary size-4' aria-hidden='true' />
        <h2 className='font-semibold'>{t('Application history')}</h2>
      </div>
      <ol className='mt-4 space-y-3'>
        {detail.events.map((event) => (
          <li key={event.id} className='rounded-lg border px-3 py-2 text-sm'>
            <div className='flex flex-wrap items-center justify-between gap-2'>
              <span className='font-medium'>
                {event.from_status === 0
                  ? t('Invoice application submitted')
                  : t('Status changed from {{from}} to {{to}}', {
                      from: t(
                        INVOICE_STATUS_META[event.from_status as InvoiceStatus]
                          .labelKey
                      ),
                      to: t(INVOICE_STATUS_META[event.to_status].labelKey),
                    })}
              </span>
              <span className='text-muted-foreground text-xs'>
                {formatInvoiceTimestamp(event.created_time)}
              </span>
            </div>
            {event.reason && (
              <p className='text-muted-foreground mt-1'>{event.reason}</p>
            )}
            {admin && event.operator_id ? (
              <p className='text-muted-foreground mt-1 text-xs'>
                {t('Operator UID: {{id}}', { id: event.operator_id })}
              </p>
            ) : null}
          </li>
        ))}
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

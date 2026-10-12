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
import { Loader2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { getUserFacingErrorMessage } from '@/lib/user-facing-error'

import { updateInvoiceStatus } from '../api'
import { formatInvoiceAmount, INVOICE_STATUS_META } from '../lib/invoice-utils'
import {
  INVOICE_STATUS,
  type InvoiceRequest,
  type InvoiceStatus,
} from '../types'

type InvoiceStatusControlsProps = {
  invoice: InvoiceRequest
  fileCount: number
  onUpdated: () => Promise<void>
}

/**
 * Administrator controls that move a pending application to a final status.
 * Every final transition is summarised in a confirmation dialog first because
 * it notifies the applicant and cannot be reverted.
 */
export function InvoiceStatusControls(props: InvoiceStatusControlsProps) {
  const { t } = useTranslation()
  const [status, setStatus] = useState<InvoiceStatus>(props.invoice.status)
  const [rejectionReason, setRejectionReason] = useState('')
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const hasFiles = props.fileCount > 0
  const reason = rejectionReason.trim()
  const issuing = status === INVOICE_STATUS.ISSUED
  const rejecting = status === INVOICE_STATUS.REJECTED
  const ready = (issuing && hasFiles) || (rejecting && reason !== '')

  if (props.invoice.status !== INVOICE_STATUS.PENDING) {
    return (
      <p className='text-muted-foreground text-sm'>
        {t('Final invoice statuses cannot be changed')}
      </p>
    )
  }

  const confirm = async () => {
    if (saving || !ready) return
    setSaving(true)
    try {
      await updateInvoiceStatus(
        props.invoice.id,
        status,
        rejecting ? reason : ''
      )
      toast.success(t('Invoice status updated'))
      setConfirmOpen(false)
      await props.onUpdated()
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setSaving(false)
    }
  }

  const taxFee = (Number(props.invoice.tax_fee_cents || 0) / 100).toFixed(2)
  const summary = (
    <dl className='bg-muted/30 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 rounded-lg border p-3 text-sm'>
      <dt className='text-muted-foreground'>{t('Invoice title')}</dt>
      <dd className='break-all'>{props.invoice.company_name || '—'}</dd>
      <dt className='text-muted-foreground'>{t('Total amount')}</dt>
      <dd>{formatInvoiceAmount(props.invoice.total_money)}</dd>
      {issuing ? (
        <>
          <dt className='text-muted-foreground'>{t('Invoice files')}</dt>
          <dd>{props.fileCount}</dd>
        </>
      ) : (
        <>
          <dt className='text-muted-foreground'>{t('Rejection reason')}</dt>
          <dd className='break-all whitespace-pre-wrap'>{reason}</dd>
        </>
      )}
    </dl>
  )
  let consequence = t(
    'The applicant will receive the issued-invoice email with the uploaded files attached when user notifications are enabled. Invoice files are locked after the application is issued.'
  )
  if (rejecting) {
    consequence =
      Number(props.invoice.tax_fee_cents || 0) > 0
        ? t(
            "The selected orders will be released and the tax fee of ¥{{amount}} will be refunded to the applicant's balance. The applicant will be notified by email when rejection notifications are enabled.",
            { amount: taxFee }
          )
        : t(
            'The selected orders will be released. The applicant will be notified by email when rejection notifications are enabled.'
          )
  }

  return (
    <div className='space-y-2'>
      <div className='flex flex-wrap items-center gap-2'>
        <Label
          htmlFor='invoice-application-status'
          className='whitespace-nowrap'
        >
          {t('Application status')}
        </Label>
        <Select
          value={String(status)}
          onValueChange={(value) => {
            if (value !== null) setStatus(Number(value) as InvoiceStatus)
          }}
        >
          <SelectTrigger id='invoice-application-status' className='w-36'>
            <SelectValue>{t(INVOICE_STATUS_META[status].labelKey)}</SelectValue>
          </SelectTrigger>
          <SelectContent
            side='bottom'
            align='start'
            sideOffset={6}
            alignItemWithTrigger={false}
          >
            <SelectItem value={String(INVOICE_STATUS.PENDING)} disabled>
              {t(INVOICE_STATUS_META[INVOICE_STATUS.PENDING].labelKey)}
            </SelectItem>
            <SelectItem
              value={String(INVOICE_STATUS.ISSUED)}
              disabled={!hasFiles}
            >
              {t(INVOICE_STATUS_META[INVOICE_STATUS.ISSUED].labelKey)}
            </SelectItem>
            <SelectItem value={String(INVOICE_STATUS.REJECTED)}>
              {t(INVOICE_STATUS_META[INVOICE_STATUS.REJECTED].labelKey)}
            </SelectItem>
          </SelectContent>
        </Select>
        <Button
          onClick={() => setConfirmOpen(true)}
          disabled={saving || !ready}
        >
          {saving && <Loader2 className='size-4 animate-spin' />}
          {t('Update status')}
        </Button>
      </div>
      {!hasFiles && (
        <p className='text-muted-foreground text-xs'>
          {t(
            'Upload at least one invoice file before marking the application as issued.'
          )}
        </p>
      )}
      {rejecting && (
        <div className='space-y-1.5 sm:w-[420px]'>
          <Label htmlFor='invoice-rejection-reason'>
            {t('Rejection reason')}
          </Label>
          <Textarea
            id='invoice-rejection-reason'
            value={rejectionReason}
            onChange={(event) => setRejectionReason(event.target.value)}
            maxLength={500}
            rows={3}
            aria-required='true'
          />
        </div>
      )}
      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={(open) => {
          if (!saving) setConfirmOpen(open)
        }}
        title={
          rejecting
            ? t('Reject this invoice application?')
            : t('Mark this invoice application as issued?')
        }
        desc={
          <div className='space-y-3'>
            {summary}
            <p>{consequence}</p>
          </div>
        }
        confirmText={rejecting ? t('Confirm rejection') : t('Confirm issue')}
        destructive={rejecting}
        disabled={!ready}
        isLoading={saving}
        handleConfirm={() => void confirm()}
      />
    </div>
  )
}

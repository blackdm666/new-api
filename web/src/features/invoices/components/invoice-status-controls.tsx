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
import { CheckCircle2, Loader2, XCircle } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { getUserFacingErrorMessage } from '@/lib/user-facing-error'

import { updateInvoiceStatus } from '../api'
import { formatInvoiceAmount } from '../lib/invoice-utils'
import { INVOICE_STATUS, type InvoiceRequest } from '../types'

type InvoiceStatusControlsProps = {
  invoice: InvoiceRequest
  fileCount: number
  onUpdated: () => Promise<void>
}

type FinalAction = 'issue' | 'reject'

/**
 * Administrator actions that move a pending application to a final status.
 * Each action opens a confirmation dialog first because it notifies the
 * applicant and cannot be reverted; rejecting asks for the reason there.
 */
export function InvoiceStatusControls(props: InvoiceStatusControlsProps) {
  const { t } = useTranslation()
  const [action, setAction] = useState<FinalAction | null>(null)
  const [rejectionReason, setRejectionReason] = useState('')
  const [saving, setSaving] = useState(false)
  const hasFiles = props.fileCount > 0
  const reason = rejectionReason.trim()
  const rejecting = action === 'reject'
  const ready = rejecting ? reason !== '' : hasFiles

  if (props.invoice.status !== INVOICE_STATUS.PENDING) {
    return (
      <p className='text-muted-foreground text-sm'>
        {t('Final invoice statuses cannot be changed')}
      </p>
    )
  }

  const confirm = async () => {
    if (saving || !action || !ready) return
    setSaving(true)
    try {
      await updateInvoiceStatus(
        props.invoice.id,
        rejecting ? INVOICE_STATUS.REJECTED : INVOICE_STATUS.ISSUED,
        rejecting ? reason : ''
      )
      toast.success(t('Invoice status updated'))
      setAction(null)
      setRejectionReason('')
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
      {!rejecting && (
        <>
          <dt className='text-muted-foreground'>{t('Invoice files')}</dt>
          <dd>{props.fileCount}</dd>
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
        <Button
          onClick={() => setAction('issue')}
          disabled={saving || !hasFiles}
        >
          {saving && !rejecting ? (
            <Loader2 className='size-4 animate-spin' aria-hidden='true' />
          ) : (
            <CheckCircle2 className='size-4' aria-hidden='true' />
          )}
          {t('Mark as issued')}
        </Button>
        <Button
          variant='outline'
          className='text-destructive hover:text-destructive'
          onClick={() => setAction('reject')}
          disabled={saving}
        >
          <XCircle className='size-4' aria-hidden='true' />
          {t('Reject application')}
        </Button>
      </div>
      {!hasFiles && (
        <p className='text-muted-foreground text-xs'>
          {t(
            'Upload at least one invoice file before marking the application as issued.'
          )}
        </p>
      )}
      <ConfirmDialog
        open={action !== null}
        onOpenChange={(open) => {
          if (!open && !saving) setAction(null)
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
      >
        {rejecting && (
          <div className='space-y-1.5'>
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
              autoFocus
            />
          </div>
        )}
      </ConfirmDialog>
    </div>
  )
}

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
import { Download, Eye, FileText, Loader2, Trash2, Upload } from 'lucide-react'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import { formatNumber } from '@/lib/format'
import { getUserFacingErrorMessage } from '@/lib/user-facing-error'

import {
  deleteInvoiceFile,
  downloadInvoiceFile,
  uploadInvoiceFile,
} from '../api'
import {
  formatInvoiceTimestamp,
  isPreviewableInvoiceFile,
} from '../lib/invoice-utils'
import { INVOICE_STATUS, type InvoiceFile, type InvoiceStatus } from '../types'

const DEFAULT_ACCEPT = '.jpg,.jpeg,.png,.webp,.pdf'

export type InvoiceFileUploadLimits = {
  /** Lowercase extensions without dots. Empty means the server decides. */
  allowedExts: string[]
  maxCount: number
  /** Bytes per file; 0 means the server decides. */
  maxSize: number
}

type InvoiceFilesSectionProps = {
  invoiceId: number
  files: InvoiceFile[]
  admin: boolean
  invoiceStatus: InvoiceStatus
  redacted: boolean
  locale?: string
  uploadLimits?: InvoiceFileUploadLimits
  onChanged: () => Promise<void>
}

export function InvoiceFilesSection(props: InvoiceFilesSectionProps) {
  const { t } = useTranslation()
  const inputRef = useRef<HTMLInputElement>(null)
  const [busy, setBusy] = useState(false)
  const [progress, setProgress] = useState<{
    current: number
    total: number
  } | null>(null)
  const [pendingDelete, setPendingDelete] = useState<InvoiceFile | null>(null)
  const limits = props.uploadLimits
  const allowedExts = limits?.allowedExts ?? []
  const maxSizeMiB = limits ? limits.maxSize / 1024 / 1024 : 0
  const remainingSlots =
    limits && limits.maxCount > 0
      ? Math.max(0, limits.maxCount - props.files.length)
      : undefined
  const canUpload =
    props.admin &&
    !props.redacted &&
    props.invoiceStatus === INVOICE_STATUS.PENDING
  const canDelete =
    props.admin && props.invoiceStatus === INVOICE_STATUS.PENDING

  const upload = async (selection: File[]) => {
    if (inputRef.current) inputRef.current.value = ''
    if (selection.length === 0 || busy) return
    if (remainingSlots !== undefined && selection.length > remainingSlots) {
      toast.error(
        t(
          'Each application can have at most {{max}} invoice files. You can upload {{remaining}} more.',
          { max: limits?.maxCount, remaining: remainingSlots }
        )
      )
      return
    }
    const rejected = selection.filter((file) => {
      const extension = file.name.includes('.')
        ? (file.name.split('.').pop() ?? '').toLowerCase()
        : ''
      const typeAllowed =
        allowedExts.length === 0 || allowedExts.includes(extension)
      const sizeAllowed = !limits?.maxSize || file.size <= limits.maxSize
      return !typeAllowed || !sizeAllowed
    })
    if (rejected.length > 0) {
      toast.error(
        t(
          'These files cannot be uploaded: {{names}}. Allowed types: {{types}}; maximum {{size}} MiB per file.',
          {
            names: rejected.map((file) => file.name).join(', '),
            types: allowedExts.join(', ') || '—',
            size: formatNumber(maxSizeMiB, props.locale),
          }
        )
      )
      return
    }

    setBusy(true)
    const toastId = toast.loading(
      t('Uploading invoice files ({{current}}/{{total}})', {
        current: 1,
        total: selection.length,
      })
    )
    let uploaded = 0
    try {
      for (const file of selection) {
        setProgress({ current: uploaded + 1, total: selection.length })
        toast.loading(
          t('Uploading invoice files ({{current}}/{{total}})', {
            current: uploaded + 1,
            total: selection.length,
          }),
          { id: toastId, description: file.name }
        )
        await uploadInvoiceFile(props.invoiceId, file)
        uploaded += 1
      }
      toast.success(
        selection.length === 1
          ? t('Invoice file uploaded')
          : t('{{n}} invoice files uploaded', { n: uploaded }),
        { id: toastId, description: undefined }
      )
    } catch (error) {
      const message = getUserFacingErrorMessage(error)
      toast.error(
        uploaded > 0
          ? t('Uploaded {{uploaded}} of {{total}} files. {{error}}', {
              uploaded,
              total: selection.length,
              error: message,
            })
          : message,
        { id: toastId, description: undefined }
      )
    } finally {
      setBusy(false)
      setProgress(null)
      if (uploaded > 0) await props.onChanged()
    }
  }

  const removeFile = async () => {
    if (!pendingDelete || busy) return
    setBusy(true)
    try {
      await deleteInvoiceFile(props.invoiceId, pendingDelete.id)
      toast.success(t('Invoice file deleted'))
      setPendingDelete(null)
      await props.onChanged()
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setBusy(false)
    }
  }

  const download = async (file: InvoiceFile, inline = false) => {
    try {
      await downloadInvoiceFile(props.invoiceId, file, inline)
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    }
  }

  let fileDescription = t('Download the issued electronic invoice here')
  if (props.invoiceStatus === INVOICE_STATUS.PENDING) {
    fileDescription = props.admin
      ? t('Upload the final PDF or image before marking as issued')
      : t('The electronic invoice will be available here after it is issued.')
  } else if (props.invoiceStatus === INVOICE_STATUS.ISSUED) {
    fileDescription = t(
      'Issued invoice files are locked. Use a new application for corrections.'
    )
  }

  let uploadLabel = t('Upload invoice file')
  if (progress) {
    uploadLabel = t('Uploading {{current}}/{{total}}', progress)
  }

  return (
    <section className='bg-card rounded-2xl border p-4 sm:p-5'>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <div>
          <h2 className='font-semibold'>{t('Issued invoice files')}</h2>
          <p className='text-muted-foreground mt-1 text-xs'>
            {fileDescription}
          </p>
          {canUpload && limits && (
            <p className='text-muted-foreground mt-1 text-xs'>
              {t(
                'Allowed types: {{types}}; up to {{max}} files, {{size}} MiB each.',
                {
                  types: allowedExts.join(', ') || '—',
                  max: limits.maxCount,
                  size: formatNumber(maxSizeMiB, props.locale),
                }
              )}
            </p>
          )}
        </div>
        {canUpload && (
          <>
            <input
              ref={inputRef}
              type='file'
              multiple
              accept={
                allowedExts.length > 0
                  ? allowedExts.map((extension) => `.${extension}`).join(',')
                  : DEFAULT_ACCEPT
              }
              className='hidden'
              aria-label={t('Upload invoice file')}
              onChange={(event) => void upload([...(event.target.files ?? [])])}
            />
            <Button
              variant='outline'
              disabled={busy || remainingSlots === 0}
              onClick={() => inputRef.current?.click()}
            >
              {busy ? (
                <Loader2 className='size-4 animate-spin' />
              ) : (
                <Upload className='size-4' />
              )}
              {uploadLabel}
            </Button>
          </>
        )}
      </div>
      {canUpload && remainingSlots === 0 && (
        <p className='text-muted-foreground mt-2 text-xs'>
          {t('The maximum of {{max}} invoice files has been reached.', {
            max: limits?.maxCount,
          })}
        </p>
      )}
      {props.files.length === 0 ? (
        <div className='text-muted-foreground mt-4 rounded-lg border border-dashed px-4 py-10 text-center text-sm'>
          {t('No invoice files')}
        </div>
      ) : (
        <div className='mt-4 space-y-2'>
          {props.files.map((file) => (
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
                  {formatInvoiceTimestamp(file.created_time, props.locale)}
                </div>
              </div>
              {isPreviewableInvoiceFile(file.mime_type) && (
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
              {canDelete && (
                <Button
                  variant='ghost'
                  size='sm'
                  className='text-destructive hover:text-destructive'
                  disabled={busy}
                  aria-label={t('Delete invoice file {{name}}', {
                    name: file.file_name,
                  })}
                  onClick={() => setPendingDelete(file)}
                >
                  <Trash2 className='size-4' />
                </Button>
              )}
            </div>
          ))}
        </div>
      )}
      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(open) => {
          if (!open && !busy) setPendingDelete(null)
        }}
        title={t('Delete this invoice file?')}
        desc={pendingDelete?.file_name ?? ''}
        confirmText={t('Delete')}
        destructive
        isLoading={busy}
        handleConfirm={() => void removeFile()}
      />
    </section>
  )
}

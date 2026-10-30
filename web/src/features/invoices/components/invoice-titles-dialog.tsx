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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Pencil, Plus, Star, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Dialog } from '@/components/dialog'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { getUserFacingErrorMessage } from '@/lib/user-facing-error'

import {
  deleteInvoiceTitle,
  fetchInvoiceTitles,
  invoiceQueryKeys,
  saveInvoiceTitle,
} from '../api'
import {
  buildInvoiceTitleFields,
  INVOICE_REGION_LABEL_KEYS,
  invoiceTitleToFormValues,
  isInvoiceTitleComplete,
  type InvoiceTitleFormValues,
} from '../lib/invoice-title'
import type { InvoiceRegion, InvoiceTitle, InvoiceTitlePayload } from '../types'
import { InvoiceRegionChoice, InvoiceTitleFields } from './invoice-title-fields'

/** Matches the server limit on saved titles per account. */
export const MAX_INVOICE_TITLES = 20

type InvoiceTitlesDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function InvoiceTitlesDialog(props: InvoiceTitlesDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState<InvoiceTitle | 'new' | null>(null)
  const [deleting, setDeleting] = useState<InvoiceTitle | null>(null)
  const titlesQuery = useQuery({
    queryKey: invoiceQueryKeys.titles(),
    queryFn: fetchInvoiceTitles,
    enabled: props.open,
    retry: false,
    meta: { errorToast: false },
  })
  const refreshTitles = () =>
    queryClient.invalidateQueries({ queryKey: invoiceQueryKeys.titles() })
  const saveMutation = useMutation({
    mutationFn: (input: { payload: InvoiceTitlePayload; id?: number }) =>
      saveInvoiceTitle(input.payload, input.id),
    onSuccess: () => {
      toast.success(t('Title saved'))
      setEditing(null)
      void refreshTitles()
    },
    onError: (error) => toast.error(getUserFacingErrorMessage(error)),
  })
  const deleteMutation = useMutation({
    mutationFn: (id: number) => deleteInvoiceTitle(id),
    onSuccess: () => {
      toast.success(t('Title deleted'))
      setDeleting(null)
      void refreshTitles()
    },
    onError: (error) => toast.error(getUserFacingErrorMessage(error)),
  })
  const titles = titlesQuery.data ?? []

  let content: React.ReactNode
  if (editing !== null) {
    content = (
      <InvoiceTitleEditor
        title={editing === 'new' ? undefined : editing}
        saving={saveMutation.isPending}
        onCancel={() => setEditing(null)}
        onSave={(payload) =>
          saveMutation.mutate({
            payload,
            id: editing === 'new' ? undefined : editing.id,
          })
        }
      />
    )
  } else if (titlesQuery.isError) {
    content = (
      <ErrorState
        className='min-h-[200px]'
        title={t('Failed to load saved titles')}
        description={getUserFacingErrorMessage(titlesQuery.error)}
        onRetry={() => void titlesQuery.refetch()}
      />
    )
  } else if (titlesQuery.isPending) {
    content = (
      <LoadingState
        className='min-h-[200px]'
        message={t('Loading saved titles...')}
      />
    )
  } else {
    content = (
      <div className='space-y-3'>
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <p className='text-muted-foreground text-xs'>
            {t('{{count}} / {{max}} saved', {
              count: titles.length,
              max: MAX_INVOICE_TITLES,
            })}
          </p>
          <Button
            size='sm'
            disabled={titles.length >= MAX_INVOICE_TITLES}
            onClick={() => setEditing('new')}
          >
            <Plus className='size-4' />
            {t('Add title')}
          </Button>
        </div>
        {titles.length === 0 ? (
          <EmptyState
            title={t('No saved titles yet')}
            description={t(
              'Save the billing details you use often, then pick them when you apply for an invoice.'
            )}
          />
        ) : (
          <ul className='divide-y rounded-lg border'>
            {titles.map((title) => (
              <li
                key={title.id}
                className='flex flex-wrap items-center gap-3 px-3 py-3'
              >
                <div className='min-w-0 flex-1 space-y-1'>
                  <div className='flex flex-wrap items-center gap-1.5'>
                    <span className='truncate font-medium'>
                      {title.company_name}
                    </span>
                    {title.is_default && (
                      <Badge variant='secondary'>{t('Default')}</Badge>
                    )}
                  </div>
                  <p className='text-muted-foreground text-xs'>
                    {t(INVOICE_REGION_LABEL_KEYS[title.region])}
                    {title.tax_number ? ` · ${title.tax_number}` : ''}
                  </p>
                </div>
                <div className='flex flex-wrap gap-1'>
                  {!title.is_default && (
                    <Button
                      variant='ghost'
                      size='sm'
                      disabled={saveMutation.isPending}
                      onClick={() =>
                        saveMutation.mutate({
                          id: title.id,
                          payload: {
                            ...buildInvoiceTitleFields(
                              title.region,
                              invoiceTitleToFormValues(title)
                            ),
                            region: title.region,
                            is_default: true,
                          },
                        })
                      }
                    >
                      <Star className='size-4' />
                      {t('Set as default')}
                    </Button>
                  )}
                  <Button
                    variant='ghost'
                    size='sm'
                    aria-label={t('Edit {{name}}', {
                      name: title.company_name,
                    })}
                    onClick={() => setEditing(title)}
                  >
                    <Pencil className='size-4' />
                    {t('Edit')}
                  </Button>
                  <Button
                    variant='ghost'
                    size='sm'
                    className='text-destructive'
                    aria-label={t('Delete {{name}}', {
                      name: title.company_name,
                    })}
                    onClick={() => setDeleting(title)}
                  >
                    <Trash2 className='size-4' />
                    {t('Delete')}
                  </Button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>
    )
  }

  return (
    <>
      <Dialog
        open={props.open}
        onOpenChange={(open) => {
          if (!open) setEditing(null)
          props.onOpenChange(open)
        }}
        title={t('Saved titles')}
        description={t(
          'Save the billing details you use often, then pick them when you apply for an invoice.'
        )}
      >
        {content}
      </Dialog>
      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(open) => {
          if (!open) setDeleting(null)
        }}
        title={t('Delete saved title?')}
        desc={t(
          'Submitted invoice applications are not affected. This cannot be undone.'
        )}
        destructive
        confirmText={t('Delete')}
        isLoading={deleteMutation.isPending}
        handleConfirm={() => {
          if (deleting) deleteMutation.mutate(deleting.id)
        }}
      />
    </>
  )
}

type InvoiceTitleEditorProps = {
  title?: InvoiceTitle
  saving: boolean
  onCancel: () => void
  onSave: (payload: InvoiceTitlePayload) => void
}

function InvoiceTitleEditor(props: InvoiceTitleEditorProps) {
  const { t } = useTranslation()
  const [region, setRegion] = useState<InvoiceRegion>(
    props.title?.region ?? 'mainland'
  )
  const [values, setValues] = useState<InvoiceTitleFormValues>(
    invoiceTitleToFormValues(props.title)
  )
  const [isDefault, setIsDefault] = useState(props.title?.is_default ?? false)
  const [attempted, setAttempted] = useState(false)

  const save = () => {
    setAttempted(true)
    if (!isInvoiceTitleComplete(region, values)) return
    props.onSave({
      ...buildInvoiceTitleFields(region, values),
      region,
      is_default: isDefault,
    })
  }

  return (
    <div className='space-y-4'>
      <InvoiceRegionChoice
        value={region}
        onChange={setRegion}
        idPrefix='invoice-title'
      />
      <div className='grid min-w-0 gap-4 sm:grid-cols-2'>
        <InvoiceTitleFields
          region={region}
          values={values}
          attempted={attempted}
          idPrefix='invoice-title'
          onChange={(key, value) =>
            setValues((current) => ({ ...current, [key]: value }))
          }
        />
      </div>
      <Label
        htmlFor='invoice-title-default'
        className='flex items-center gap-2 font-normal'
      >
        <Checkbox
          id='invoice-title-default'
          checked={isDefault}
          onCheckedChange={(checked) => setIsDefault(checked === true)}
        />
        {t('Use as the default for this invoice type')}
      </Label>
      <div className='flex justify-end gap-2'>
        <Button variant='outline' onClick={props.onCancel}>
          {t('Cancel')}
        </Button>
        <Button disabled={props.saving} onClick={save}>
          {t('Save')}
        </Button>
      </div>
    </div>
  )
}

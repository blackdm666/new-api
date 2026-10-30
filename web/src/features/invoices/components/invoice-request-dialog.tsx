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
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Mail, MailWarning } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'
import { getPaymentMethodName } from '@/features/wallet/lib/billing'
import { toIntlLocale } from '@/i18n/languages'
import { getUserFacingErrorMessage } from '@/lib/user-facing-error'
import { cn } from '@/lib/utils'

import {
  createInvoiceRequest,
  fetchEligibleInvoiceOrders,
  fetchInvoiceConfig,
  fetchInvoiceTitles,
  invoiceQueryKeys,
} from '../api'
import {
  MAX_INVOICE_ORDER_SELECTION,
  selectInvoiceOrderIds,
} from '../lib/invoice-selection'
import {
  buildInvoiceTitleFields,
  invoiceTitleToFormValues,
  isInvoiceTitleComplete,
  pickDefaultInvoiceTitle,
  type InvoiceTitleFormValues,
} from '../lib/invoice-title'
import {
  calculateInvoiceTaxFee,
  formatInvoiceTimestamp,
  getInvoiceAmountShortfall,
} from '../lib/invoice-utils'
import type {
  EligibleTopUpOrder,
  InvoiceConfig,
  InvoiceRegion,
  InvoiceTitle,
} from '../types'
import { InvoiceSampleDialog } from './invoice-samples'
import {
  InvoiceFormField,
  InvoiceRegionChoice,
  InvoiceTitleFields,
} from './invoice-title-fields'

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: () => void
}

export function InvoiceRequestDialog(props: Props) {
  if (!props.open) return null
  return (
    <OpenInvoiceRequestDialog
      onOpenChange={props.onOpenChange}
      onCreated={props.onCreated}
    />
  )
}

function OpenInvoiceRequestDialog(props: Omit<Props, 'open'>) {
  const { t } = useTranslation()
  // The dialog must never fall back to placeholder limits: it stays in a
  // loading state until both the configuration and the orders are known.
  const configQuery = useQuery({
    queryKey: invoiceQueryKeys.config(),
    queryFn: fetchInvoiceConfig,
    staleTime: 0,
    retry: false,
    meta: { errorToast: false },
  })
  const ordersQuery = useQuery({
    queryKey: invoiceQueryKeys.eligibleOrders(),
    queryFn: fetchEligibleInvoiceOrders,
    staleTime: 0,
    gcTime: 0,
    retry: false,
    meta: { errorToast: false },
  })
  // Saved titles only prefill the form, so failing to load them never blocks
  // the application.
  const titlesQuery = useQuery({
    queryKey: invoiceQueryKeys.titles(),
    queryFn: fetchInvoiceTitles,
    retry: false,
    meta: { errorToast: false },
  })
  const config = configQuery.data
  const failedQuery = [configQuery, ordersQuery].find(
    (query) => query.isError && !query.isFetching
  )

  let description = t(
    'Select paid top-up orders, submit invoice information, and download issued invoices'
  )
  if (config) {
    const rate = (config.tax_rate_basis_points / 100).toFixed(2)
    description = [
      t(
        'We respond within {{days}} working days of receiving your application. The minimum invoice amount is ¥{{amount}}, and multiple paid top-up orders can be combined. The issued invoice and notification will be sent to your account email, and the invoice can also be downloaded from the console.',
        {
          days: config.response_working_days,
          amount: (config.minimum_amount_cents / 100).toFixed(2),
        }
      ),
      t(
        'Mainland China invoices: a {{rate}}% tax fee is deducted from your balance when you submit and is refunded in full if no invoice is issued (the application is withdrawn, rejected, or expires). Hong Kong invoices have no tax fee.',
        { rate }
      ),
      config.pending_expiry_days > 0
        ? t(
            'Applications that are not processed within {{days}} days expire automatically.',
            { days: config.pending_expiry_days }
          )
        : '',
    ]
      .filter(Boolean)
      .join(' ')
  }

  let content: React.ReactNode
  if (failedQuery) {
    content = (
      <>
        <ErrorState
          className='min-h-[240px]'
          title={t('Failed to load invoice information')}
          description={getUserFacingErrorMessage(failedQuery.error)}
          onRetry={() => {
            if (configQuery.isError) void configQuery.refetch()
            if (ordersQuery.isError) void ordersQuery.refetch()
          }}
        />
        <DialogFooter>
          <Button variant='outline' onClick={() => props.onOpenChange(false)}>
            {t('Cancel')}
          </Button>
        </DialogFooter>
      </>
    )
  } else if (!config || !ordersQuery.data || titlesQuery.isPending) {
    content = (
      <>
        <LoadingState
          className='min-h-[240px]'
          message={t('Loading invoice information...')}
        />
        <DialogFooter>
          <Button variant='outline' onClick={() => props.onOpenChange(false)}>
            {t('Cancel')}
          </Button>
        </DialogFooter>
      </>
    )
  } else {
    content = (
      <InvoiceRequestForm
        config={config}
        orders={ordersQuery.data}
        titles={titlesQuery.data ?? []}
        onCancel={() => props.onOpenChange(false)}
        onCreated={() => {
          props.onOpenChange(false)
          props.onCreated()
        }}
      />
    )
  }

  return (
    <Dialog open onOpenChange={props.onOpenChange}>
      <DialogContent
        className='flex max-h-[90vh] flex-col sm:max-w-3xl'
        closeLabel={t('Close')}
      >
        <DialogHeader>
          <DialogTitle>{t('New invoice application')}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        {content}
      </DialogContent>
    </Dialog>
  )
}

type InvoiceRequestFormProps = {
  config: InvoiceConfig
  orders: EligibleTopUpOrder[]
  titles: InvoiceTitle[]
  onCancel: () => void
  onCreated: () => void
}

function InvoiceRequestForm(props: InvoiceRequestFormProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const orders = props.orders
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [region, setRegion] = useState<InvoiceRegion>('mainland')
  const initialTitle = pickDefaultInvoiceTitle(props.titles, 'mainland')
  const [titleId, setTitleId] = useState(initialTitle?.id ?? 0)
  const [form, setForm] = useState<InvoiceTitleFormValues>(
    invoiceTitleToFormValues(initialTitle)
  )
  const [remark, setRemark] = useState('')
  const [saveTitle, setSaveTitle] = useState(false)
  const [sampleOpen, setSampleOpen] = useState(false)
  const [sampleRegion, setSampleRegion] = useState<InvoiceRegion>('mainland')
  const [submitting, setSubmitting] = useState(false)
  const [attempted, setAttempted] = useState(false)
  const minimumAmount = props.config.minimum_amount_cents / 100
  const taxRateBasisPoints = props.config.tax_rate_basis_points
  const availableBalance = props.config.available_balance_cents / 100
  const emailBound = props.config.email_bound

  const total = useMemo(
    () =>
      orders.reduce(
        (sum, order) =>
          selected.has(order.id) ? sum + Number(order.money || 0) : sum,
        0
      ),
    [orders, selected]
  )
  // Hong Kong invoices carry no tax fee.
  const taxFee = useMemo(
    () =>
      region === 'hk' ? 0 : calculateInvoiceTaxFee(total, taxRateBasisPoints),
    [region, taxRateBasisPoints, total]
  )
  const balanceInsufficient = taxFee > availableBalance
  const regionTitles = props.titles.filter((title) => title.region === region)
  const canSubmit =
    emailBound &&
    selected.size > 0 &&
    total >= minimumAmount &&
    !balanceInsufficient &&
    isInvoiceTitleComplete(region, form)

  const changeRegion = (next: InvoiceRegion) => {
    if (next === region) return
    const nextTitle = pickDefaultInvoiceTitle(props.titles, next)
    setRegion(next)
    setTitleId(nextTitle?.id ?? 0)
    setForm(invoiceTitleToFormValues(nextTitle))
    setAttempted(false)
  }

  const applySavedTitle = (id: number) => {
    setTitleId(id)
    setForm(
      invoiceTitleToFormValues(props.titles.find((title) => title.id === id))
    )
  }

  const submit = async () => {
    if (submitting) return
    setAttempted(true)
    if (!canSubmit) return
    setSubmitting(true)
    try {
      const fields = buildInvoiceTitleFields(region, form)
      await createInvoiceRequest(
        region === 'hk'
          ? {
              region,
              company_name: fields.company_name,
              remark: remark.trim(),
              topup_order_ids: [...selected],
              save_title: saveTitle,
            }
          : {
              region,
              ...fields,
              remark: remark.trim(),
              topup_order_ids: [...selected],
              save_title: saveTitle,
            }
      )
      toast.success(t('Invoice application submitted'))
      props.onCreated()
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setSubmitting(false)
    }
  }

  let orderContent: React.ReactNode
  if (orders.length === 0) {
    orderContent = (
      <p className='text-muted-foreground px-4 py-8 text-center text-sm'>
        {t('No eligible paid orders')}
      </p>
    )
  } else {
    orderContent = orders.map((order) => {
      const checked = selected.has(order.id)
      const selectionLimitReached =
        !checked && selected.size >= MAX_INVOICE_ORDER_SELECTION
      return (
        <label
          htmlFor={`invoice-order-${order.id}`}
          key={order.id}
          className={cn(
            'hover:bg-accent/40 flex w-full cursor-pointer items-center gap-3 border-b px-3 py-3 text-left last:border-0',
            checked && 'bg-primary/5'
          )}
        >
          <Checkbox
            id={`invoice-order-${order.id}`}
            checked={checked}
            disabled={selectionLimitReached}
            onCheckedChange={(nextChecked) =>
              setSelected((current) => {
                const next = new Set(current)
                if (nextChecked && next.size < MAX_INVOICE_ORDER_SELECTION) {
                  next.add(order.id)
                } else next.delete(order.id)
                return next
              })
            }
          />
          <div className='min-w-0 flex-1'>
            <div className='truncate font-mono text-xs'>
              {order.trade_no || `#${order.id}`}
            </div>
            <div className='text-muted-foreground text-xs'>
              {order.payment_method
                ? getPaymentMethodName(order.payment_method, t)
                : '—'}{' '}
              ·{' '}
              {formatInvoiceTimestamp(
                order.complete_time || order.create_time,
                locale
              )}
            </div>
          </div>
          <span className='font-semibold'>
            ¥{Number(order.money || 0).toFixed(2)}
          </span>
        </label>
      )
    })
  }

  const allSelectableSelected =
    selected.size === Math.min(orders.length, MAX_INVOICE_ORDER_SELECTION)
  return (
    <>
      <div className='min-h-0 flex-1 space-y-5 overflow-y-auto pr-1'>
        <InvoiceRegionChoice
          value={region}
          onChange={changeRegion}
          idPrefix='invoice'
          onViewSample={(next) => {
            setSampleRegion(next)
            setSampleOpen(true)
          }}
        />
        {emailBound ? (
          props.config.email && (
            <p className='text-muted-foreground flex items-center gap-2 text-sm'>
              <Mail className='size-4 shrink-0' aria-hidden='true' />
              {t('The invoice will be sent to {{email}}', {
                email: props.config.email,
              })}
            </p>
          )
        ) : (
          <Alert className='border-amber-500/40 bg-amber-500/5'>
            <MailWarning aria-hidden='true' />
            <AlertTitle>
              {t('No email address is bound to your account')}
            </AlertTitle>
            <AlertDescription className='space-y-2'>
              <p>
                {t(
                  'Issued invoices and notifications are sent by email. Bind an email address before submitting an invoice application.'
                )}
              </p>
              <Button
                size='sm'
                variant='outline'
                render={<Link to='/security' />}
              >
                {t('Bind Email')}
              </Button>
            </AlertDescription>
          </Alert>
        )}
        <section className='space-y-3'>
          <div className='flex items-center justify-between'>
            <Label>{t('Select paid top-up orders')}</Label>
            {orders.length > 0 && (
              <Button
                type='button'
                variant='ghost'
                size='sm'
                onClick={() =>
                  setSelected(
                    allSelectableSelected
                      ? new Set()
                      : selectInvoiceOrderIds(orders.map((order) => order.id))
                  )
                }
              >
                {allSelectableSelected ? t('Clear selection') : t('Select all')}
              </Button>
            )}
          </div>
          {orders.length > MAX_INVOICE_ORDER_SELECTION && (
            <p className='text-muted-foreground text-xs'>
              {t('Up to {{count}} paid orders can be selected at a time.', {
                count: MAX_INVOICE_ORDER_SELECTION,
              })}
            </p>
          )}
          <div className='max-h-56 overflow-y-auto rounded-lg border'>
            {orderContent}
          </div>
          <div className='flex flex-wrap items-end justify-between gap-2 text-sm'>
            <p className='text-muted-foreground'>
              {t('Minimum invoice amount')}: ¥{minimumAmount.toFixed(2)}
            </p>
            <div className='text-right'>
              {t('Selected total')}:{' '}
              <span className='text-primary text-lg font-semibold'>
                ¥{total.toFixed(2)}
              </span>
            </div>
          </div>
          {selected.size > 0 && total < minimumAmount && (
            <p
              className='text-destructive text-right text-xs'
              role='status'
              aria-live='polite'
            >
              {t(
                'The selected amount is below the minimum. Add ¥{{amount}} more to submit; you can select additional paid top-up orders.',
                {
                  amount: getInvoiceAmountShortfall(
                    total,
                    minimumAmount
                  ).toFixed(2),
                }
              )}
            </p>
          )}
          {selected.size > 0 && region === 'mainland' && (
            <div className='bg-muted/30 grid gap-2 rounded-lg border p-3 text-sm sm:grid-cols-3'>
              <div>
                <p className='text-muted-foreground text-xs'>
                  {t('Invoice tax rate')}
                </p>
                <p className='font-semibold'>
                  {(taxRateBasisPoints / 100).toFixed(2)}%
                </p>
              </div>
              <div>
                <p className='text-muted-foreground text-xs'>
                  {t('Tax fee payable')}
                </p>
                <p className='font-semibold'>¥{taxFee.toFixed(2)}</p>
              </div>
              <div>
                <p className='text-muted-foreground text-xs'>
                  {t('Available balance')}
                </p>
                <p className='font-semibold'>¥{availableBalance.toFixed(2)}</p>
              </div>
            </div>
          )}
          {selected.size > 0 && balanceInsufficient && (
            <p className='text-destructive text-right text-xs' role='alert'>
              {t(
                'Insufficient balance. This application requires a tax fee of ¥{{fee}}; please top up before submitting.',
                { fee: taxFee.toFixed(2) }
              )}
            </p>
          )}
        </section>

        {orders.length === 0 && (
          <p className='rounded-lg border border-amber-500/30 bg-amber-500/5 px-3 py-2 text-sm'>
            {t(
              'Billing details can be entered after you have at least one paid order available for invoicing.'
            )}
          </p>
        )}
        <fieldset
          className='grid min-w-0 gap-4 disabled:opacity-60 sm:grid-cols-2'
          disabled={orders.length === 0}
        >
          {regionTitles.length > 0 && (
            <InvoiceFormField
              id='invoice-saved-title'
              label={t('Use a saved title')}
              className='sm:col-span-2'
            >
              <NativeSelect
                id='invoice-saved-title'
                className='w-full'
                value={String(titleId)}
                onChange={(event) =>
                  applySavedTitle(Number(event.target.value))
                }
              >
                <NativeSelectOption value='0'>
                  {t('Enter new details')}
                </NativeSelectOption>
                {regionTitles.map((title) => (
                  <NativeSelectOption key={title.id} value={String(title.id)}>
                    {title.tax_number
                      ? `${title.company_name} · ${title.tax_number}`
                      : title.company_name}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </InvoiceFormField>
          )}
          <InvoiceTitleFields
            region={region}
            values={form}
            attempted={attempted}
            idPrefix='invoice'
            onChange={(key, value) =>
              setForm((current) => ({ ...current, [key]: value }))
            }
          />
          <InvoiceFormField
            id='invoice-remark'
            label={t('Remark')}
            className='sm:col-span-2'
          >
            <Textarea
              id='invoice-remark'
              value={remark}
              maxLength={2000}
              onChange={(event) => setRemark(event.target.value)}
              rows={3}
            />
          </InvoiceFormField>
          <Label
            htmlFor='invoice-save-title'
            className='flex items-center gap-2 font-normal sm:col-span-2'
          >
            <Checkbox
              id='invoice-save-title'
              checked={saveTitle}
              onCheckedChange={(checked) => setSaveTitle(checked === true)}
            />
            {t('Save these details as a saved title')}
          </Label>
        </fieldset>
      </div>
      <DialogFooter>
        <Button variant='outline' onClick={props.onCancel}>
          {t('Cancel')}
        </Button>
        <Button
          onClick={submit}
          disabled={
            !emailBound ||
            selected.size === 0 ||
            total < minimumAmount ||
            submitting ||
            balanceInsufficient
          }
        >
          {t('Submit invoice application')}
        </Button>
      </DialogFooter>
      <InvoiceSampleDialog
        open={sampleOpen}
        onOpenChange={setSampleOpen}
        region={sampleRegion}
        onRegionChange={setSampleRegion}
      />
    </>
  )
}

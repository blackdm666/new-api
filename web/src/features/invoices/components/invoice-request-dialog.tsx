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
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { getPaymentMethodName } from '@/features/wallet/lib/billing'
import { toIntlLocale } from '@/i18n/languages'
import { getUserFacingErrorMessage } from '@/lib/user-facing-error'
import { cn } from '@/lib/utils'

import {
  createInvoiceRequest,
  fetchEligibleInvoiceOrders,
  fetchInvoiceConfig,
  invoiceQueryKeys,
} from '../api'
import {
  MAX_INVOICE_ORDER_SELECTION,
  selectInvoiceOrderIds,
} from '../lib/invoice-selection'
import {
  calculateInvoiceTaxFee,
  formatInvoiceTimestamp,
  getInvoiceAmountShortfall,
  INVOICE_TAX_NUMBER_LENGTH,
  normalizeInvoiceTaxNumber,
} from '../lib/invoice-utils'
import type { EligibleTopUpOrder, InvoiceConfig } from '../types'

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: () => void
}

const EMPTY_FORM = {
  companyName: '',
  taxNumber: '',
  bankName: '',
  bankAccount: '',
  companyAddress: '',
  companyPhone: '',
  remark: '',
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
        'Invoices are issued together on day {{day}} of each month. The minimum invoice amount is ¥{{amount}}, and multiple paid top-up orders can be combined. The issued invoice and notification will be sent to your account email, and the invoice can also be downloaded from the console.',
        {
          day: config.issue_day,
          amount: (config.minimum_amount_cents / 100).toFixed(2),
        }
      ),
      t(
        'A {{rate}}% tax fee is deducted from your balance when you submit. If no invoice is issued (the application is withdrawn, rejected, or expires), the tax fee is refunded to your balance in full.',
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
  } else if (!config || !ordersQuery.data) {
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
  onCancel: () => void
  onCreated: () => void
}

function InvoiceRequestForm(props: InvoiceRequestFormProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const orders = props.orders
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [form, setForm] = useState(EMPTY_FORM)
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
  const companyError =
    attempted && form.companyName.trim() === ''
      ? t('Invoice title is required')
      : ''
  const taxNumber = normalizeInvoiceTaxNumber(form.taxNumber)
  const taxNumberError =
    attempted && taxNumber === '' ? t('Tax number is required') : ''
  const taxNumberLengthUnusual =
    taxNumber !== '' &&
    (taxNumber.length < INVOICE_TAX_NUMBER_LENGTH.min ||
      taxNumber.length > INVOICE_TAX_NUMBER_LENGTH.max)
  const taxFee = useMemo(
    () => calculateInvoiceTaxFee(total, taxRateBasisPoints),
    [taxRateBasisPoints, total]
  )
  const balanceInsufficient = taxFee > availableBalance
  const canSubmit =
    emailBound &&
    selected.size > 0 &&
    total >= minimumAmount &&
    !balanceInsufficient &&
    form.companyName.trim() !== '' &&
    taxNumber !== ''

  const submit = async () => {
    if (submitting) return
    setAttempted(true)
    if (!canSubmit) return
    setSubmitting(true)
    try {
      await createInvoiceRequest({
        company_name: form.companyName.trim(),
        tax_number: taxNumber,
        bank_name: form.bankName.trim(),
        bank_account: form.bankAccount.trim(),
        company_address: form.companyAddress.trim(),
        company_phone: form.companyPhone.trim(),
        remark: form.remark.trim(),
        topup_order_ids: [...selected],
      })
      toast.success(t('Invoice application submitted'))
      props.onCreated()
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setSubmitting(false)
    }
  }

  const field = (key: keyof typeof EMPTY_FORM, value: string) =>
    setForm((current) => ({ ...current, [key]: value }))

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
          {selected.size > 0 && (
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
          <FormField
            id='invoice-company-name'
            label={t('Invoice title')}
            required
            error={companyError}
          >
            <Input
              id='invoice-company-name'
              aria-invalid={Boolean(companyError)}
              aria-describedby={
                companyError ? 'invoice-company-name-error' : undefined
              }
              value={form.companyName}
              maxLength={255}
              onChange={(event) => field('companyName', event.target.value)}
            />
          </FormField>
          <FormField
            id='invoice-tax-number'
            label={t('Tax number')}
            required
            error={taxNumberError}
            hint={
              taxNumberLengthUnusual
                ? t(
                    'The tax number has {{length}} characters. A unified social credit code is usually 18 characters (15–20 accepted); please double-check it.',
                    { length: taxNumber.length }
                  )
                : t(
                    'A unified social credit code is usually 18 characters (15–20 accepted).'
                  )
            }
            hintTone={taxNumberLengthUnusual ? 'warning' : 'muted'}
          >
            <Input
              id='invoice-tax-number'
              aria-invalid={Boolean(taxNumberError)}
              aria-describedby={
                taxNumberError
                  ? 'invoice-tax-number-hint invoice-tax-number-error'
                  : 'invoice-tax-number-hint'
              }
              autoCapitalize='characters'
              spellCheck={false}
              value={form.taxNumber}
              maxLength={64}
              onChange={(event) => field('taxNumber', event.target.value)}
            />
          </FormField>
          <FormField id='invoice-bank-name' label={t('Bank name')}>
            <Input
              id='invoice-bank-name'
              value={form.bankName}
              maxLength={255}
              onChange={(event) => field('bankName', event.target.value)}
            />
          </FormField>
          <FormField id='invoice-bank-account' label={t('Bank account')}>
            <Input
              id='invoice-bank-account'
              value={form.bankAccount}
              maxLength={128}
              onChange={(event) => field('bankAccount', event.target.value)}
            />
          </FormField>
          <FormField id='invoice-company-address' label={t('Company address')}>
            <Input
              id='invoice-company-address'
              value={form.companyAddress}
              maxLength={512}
              onChange={(event) => field('companyAddress', event.target.value)}
            />
          </FormField>
          <FormField id='invoice-company-phone' label={t('Company phone')}>
            <Input
              id='invoice-company-phone'
              value={form.companyPhone}
              maxLength={32}
              onChange={(event) => field('companyPhone', event.target.value)}
            />
          </FormField>
          <FormField
            id='invoice-remark'
            label={t('Remark')}
            className='sm:col-span-2'
          >
            <Textarea
              id='invoice-remark'
              value={form.remark}
              maxLength={2000}
              onChange={(event) => field('remark', event.target.value)}
              rows={3}
            />
          </FormField>
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
    </>
  )
}

type FormFieldProps = {
  id: string
  label: string
  required?: boolean
  className?: string
  error?: string
  hint?: string
  hintTone?: 'muted' | 'warning'
  children: React.ReactNode
}

function FormField(props: FormFieldProps) {
  return (
    <div className={cn('space-y-1.5', props.className)}>
      <Label htmlFor={props.id}>
        {props.label}
        {props.required && <span className='text-destructive ml-1'>*</span>}
      </Label>
      {props.children}
      {props.hint && (
        <p
          id={`${props.id}-hint`}
          className={cn(
            'text-xs',
            props.hintTone === 'warning'
              ? 'text-amber-600 dark:text-amber-400'
              : 'text-muted-foreground'
          )}
        >
          {props.hint}
        </p>
      )}
      {props.error && (
        <p
          id={`${props.id}-error`}
          className='text-destructive text-xs'
          role='alert'
        >
          {props.error}
        </p>
      )}
    </div>
  )
}

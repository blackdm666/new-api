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
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

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
import { Skeleton } from '@/components/ui/skeleton'
import { Textarea } from '@/components/ui/textarea'
import { getUserFacingErrorMessage } from '@/lib/user-facing-error'
import { cn } from '@/lib/utils'

import {
  createInvoiceRequest,
  fetchEligibleInvoiceOrders,
  fetchInvoiceConfig,
} from '../api'
import {
  MAX_INVOICE_ORDER_SELECTION,
  selectInvoiceOrderIds,
} from '../lib/invoice-selection'
import {
  formatInvoiceTimestamp,
  getInvoiceAmountShortfall,
} from '../lib/invoice-utils'
import type { EligibleTopUpOrder } from '../types'

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

export function InvoiceRequestDialog({ open, onOpenChange, onCreated }: Props) {
  const { t } = useTranslation()
  const [orders, setOrders] = useState<EligibleTopUpOrder[]>([])
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [form, setForm] = useState(EMPTY_FORM)
  const [loading, setLoading] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [minimumAmount, setMinimumAmount] = useState(500)
  const [issueDay, setIssueDay] = useState(10)
  const [attempted, setAttempted] = useState(false)

  useEffect(() => {
    if (!open) {
      setOrders([])
      setSelected(new Set())
      setForm(EMPTY_FORM)
      setAttempted(false)
      return
    }
    setLoading(true)
    void Promise.all([fetchEligibleInvoiceOrders(), fetchInvoiceConfig()])
      .then(([eligibleOrders, config]) => {
        setOrders(eligibleOrders)
        setMinimumAmount(config.minimum_amount_cents / 100)
        setIssueDay(config.issue_day)
      })
      .catch((error) => toast.error(getUserFacingErrorMessage(error)))
      .finally(() => setLoading(false))
  }, [open, t])

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
      ? t('Company name is required')
      : ''
  const taxNumberError =
    attempted && form.taxNumber.trim() === '' ? t('Tax number is required') : ''
  const canSubmit =
    selected.size > 0 &&
    total >= minimumAmount &&
    form.companyName.trim() !== '' &&
    form.taxNumber.trim() !== ''

  const submit = async () => {
    if (submitting) return
    setAttempted(true)
    if (!canSubmit) return
    setSubmitting(true)
    try {
      await createInvoiceRequest({
        company_name: form.companyName.trim(),
        tax_number: form.taxNumber.trim(),
        bank_name: form.bankName.trim(),
        bank_account: form.bankAccount.trim(),
        company_address: form.companyAddress.trim(),
        company_phone: form.companyPhone.trim(),
        remark: form.remark.trim(),
        topup_order_ids: [...selected],
      })
      toast.success(t('Invoice application submitted'))
      onOpenChange(false)
      onCreated()
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setSubmitting(false)
    }
  }

  const field = (key: keyof typeof EMPTY_FORM, value: string) =>
    setForm((current) => ({ ...current, [key]: value }))

  let orderContent: React.ReactNode
  if (loading) {
    orderContent = (
      <div className='space-y-2 p-3'>
        <Skeleton className='h-12 w-full' />
        <Skeleton className='h-12 w-full' />
      </div>
    )
  } else if (orders.length === 0) {
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
              {order.payment_method || '—'} ·{' '}
              {formatInvoiceTimestamp(order.complete_time || order.create_time)}
            </div>
          </div>
          <span className='font-semibold'>
            ¥{Number(order.money || 0).toFixed(2)}
          </span>
        </label>
      )
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className='flex max-h-[90vh] flex-col sm:max-w-3xl'
        closeLabel={t('Close')}
      >
        <DialogHeader>
          <DialogTitle>{t('New invoice application')}</DialogTitle>
          <DialogDescription>
            {t(
              'Invoices are issued together on day {{day}} of each month. The minimum invoice amount is ¥{{amount}}, and multiple paid top-up orders can be combined. The issued invoice and notification will be sent to your account email, and the invoice can also be downloaded from the console.',
              { day: issueDay, amount: minimumAmount.toFixed(2) }
            )}
          </DialogDescription>
        </DialogHeader>
        <div className='min-h-0 flex-1 space-y-5 overflow-y-auto pr-1'>
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
                      selected.size ===
                        Math.min(orders.length, MAX_INVOICE_ORDER_SELECTION)
                        ? new Set()
                        : selectInvoiceOrderIds(orders.map((order) => order.id))
                    )
                  }
                >
                  {selected.size ===
                  Math.min(orders.length, MAX_INVOICE_ORDER_SELECTION)
                    ? t('Clear selection')
                    : t('Select all')}
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
          </section>

          {orders.length === 0 && !loading && (
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
              label={t('Company name')}
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
            >
              <Input
                id='invoice-tax-number'
                aria-invalid={Boolean(taxNumberError)}
                aria-describedby={
                  taxNumberError ? 'invoice-tax-number-error' : undefined
                }
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
            <FormField
              id='invoice-company-address'
              label={t('Company address')}
            >
              <Input
                id='invoice-company-address'
                value={form.companyAddress}
                maxLength={512}
                onChange={(event) =>
                  field('companyAddress', event.target.value)
                }
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
          <Button variant='outline' onClick={() => onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button
            onClick={submit}
            disabled={
              selected.size === 0 || total < minimumAmount || submitting
            }
          >
            {t('Submit invoice application')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function FormField({
  id,
  label,
  required,
  className,
  error,
  children,
}: {
  id: string
  label: string
  required?: boolean
  className?: string
  error?: string
  children: React.ReactNode
}) {
  return (
    <div className={`space-y-1.5 ${className ?? ''}`}>
      <Label htmlFor={id}>
        {label}
        {required && <span className='text-destructive ml-1'>*</span>}
      </Label>
      {children}
      {error && (
        <p id={`${id}-error`} className='text-destructive text-xs' role='alert'>
          {error}
        </p>
      )}
    </div>
  )
}

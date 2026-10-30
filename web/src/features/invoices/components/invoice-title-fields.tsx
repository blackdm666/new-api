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
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { cn } from '@/lib/utils'

import {
  INVOICE_REGION_LABEL_KEYS,
  INVOICE_REGIONS,
  type InvoiceTitleFormValues,
} from '../lib/invoice-title'
import {
  INVOICE_TAX_NUMBER_LENGTH,
  normalizeInvoiceTaxNumber,
} from '../lib/invoice-utils'
import type { InvoiceRegion } from '../types'

const INVOICE_REGION_NOTE_KEYS: Record<InvoiceRegion, string> = {
  mainland: 'Requires the invoice title and tax number. A tax fee applies.',
  hk: 'Only the recipient name is needed. No tax fee.',
}

type InvoiceRegionChoiceProps = {
  value: InvoiceRegion
  onChange: (region: InvoiceRegion) => void
  idPrefix: string
  /** Shows a "View sample" action on each card when provided. */
  onViewSample?: (region: InvoiceRegion) => void
}

export function InvoiceRegionChoice(props: InvoiceRegionChoiceProps) {
  const { t } = useTranslation()
  const labelId = `${props.idPrefix}-region-label`
  return (
    <div className='space-y-2'>
      <Label id={labelId}>{t('Invoice type')}</Label>
      <RadioGroup
        aria-labelledby={labelId}
        value={props.value}
        onValueChange={(value) => props.onChange(value as InvoiceRegion)}
        className='grid gap-2 sm:grid-cols-2'
      >
        {INVOICE_REGIONS.map((region) => (
          <div
            key={region}
            className={cn(
              'flex flex-col gap-2 rounded-lg border p-3',
              props.value === region && 'border-primary bg-primary/5'
            )}
          >
            <Label
              htmlFor={`${props.idPrefix}-region-${region}`}
              className='flex cursor-pointer items-start gap-3 font-normal'
            >
              <RadioGroupItem
                id={`${props.idPrefix}-region-${region}`}
                value={region}
                className='mt-0.5'
              />
              <span className='space-y-1'>
                <span className='block text-sm font-medium'>
                  {t(INVOICE_REGION_LABEL_KEYS[region])}
                </span>
                <span className='text-muted-foreground block text-xs leading-5'>
                  {t(INVOICE_REGION_NOTE_KEYS[region])}
                </span>
              </span>
            </Label>
            {props.onViewSample && (
              <Button
                type='button'
                variant='link'
                size='sm'
                className='h-auto self-start p-0 pl-7'
                onClick={() => props.onViewSample?.(region)}
              >
                {t('View sample')}
              </Button>
            )}
          </div>
        ))}
      </RadioGroup>
    </div>
  )
}

type InvoiceTitleFieldsProps = {
  region: InvoiceRegion
  values: InvoiceTitleFormValues
  onChange: (key: keyof InvoiceTitleFormValues, value: string) => void
  /** Show required-field errors once the user tried to submit. */
  attempted: boolean
  idPrefix: string
}

/**
 * Billing detail inputs for one invoice type. Hong Kong invoices only name
 * the recipient; mainland VAT invoices need the tax number and may carry
 * bank, address and phone details.
 */
export function InvoiceTitleFields(props: InvoiceTitleFieldsProps) {
  const { t } = useTranslation()
  const id = (name: string) => `${props.idPrefix}-${name}`
  const companyError =
    props.attempted && props.values.companyName.trim() === ''
      ? t('Invoice title is required')
      : ''

  if (props.region === 'hk') {
    return (
      <InvoiceFormField
        id={id('company-name')}
        label={t('Recipient name (company or individual)')}
        required
        error={companyError}
        hint={t('Hong Kong invoices only show the recipient name.')}
        className='sm:col-span-2'
      >
        <Input
          id={id('company-name')}
          aria-invalid={Boolean(companyError)}
          aria-describedby={
            companyError
              ? `${id('company-name')}-hint ${id('company-name')}-error`
              : `${id('company-name')}-hint`
          }
          value={props.values.companyName}
          maxLength={255}
          onChange={(event) =>
            props.onChange('companyName', event.target.value)
          }
        />
      </InvoiceFormField>
    )
  }

  const taxNumber = normalizeInvoiceTaxNumber(props.values.taxNumber)
  const taxNumberError =
    props.attempted && taxNumber === '' ? t('Tax number is required') : ''
  const taxNumberLengthUnusual =
    taxNumber !== '' &&
    (taxNumber.length < INVOICE_TAX_NUMBER_LENGTH.min ||
      taxNumber.length > INVOICE_TAX_NUMBER_LENGTH.max)
  return (
    <>
      <InvoiceFormField
        id={id('company-name')}
        label={t('Invoice title')}
        required
        error={companyError}
      >
        <Input
          id={id('company-name')}
          aria-invalid={Boolean(companyError)}
          aria-describedby={
            companyError ? `${id('company-name')}-error` : undefined
          }
          value={props.values.companyName}
          maxLength={255}
          onChange={(event) =>
            props.onChange('companyName', event.target.value)
          }
        />
      </InvoiceFormField>
      <InvoiceFormField
        id={id('tax-number')}
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
          id={id('tax-number')}
          aria-invalid={Boolean(taxNumberError)}
          aria-describedby={
            taxNumberError
              ? `${id('tax-number')}-hint ${id('tax-number')}-error`
              : `${id('tax-number')}-hint`
          }
          autoCapitalize='characters'
          spellCheck={false}
          value={props.values.taxNumber}
          maxLength={64}
          onChange={(event) => props.onChange('taxNumber', event.target.value)}
        />
      </InvoiceFormField>
      <InvoiceFormField id={id('bank-name')} label={t('Bank name')}>
        <Input
          id={id('bank-name')}
          value={props.values.bankName}
          maxLength={255}
          onChange={(event) => props.onChange('bankName', event.target.value)}
        />
      </InvoiceFormField>
      <InvoiceFormField id={id('bank-account')} label={t('Bank account')}>
        <Input
          id={id('bank-account')}
          value={props.values.bankAccount}
          maxLength={128}
          onChange={(event) =>
            props.onChange('bankAccount', event.target.value)
          }
        />
      </InvoiceFormField>
      <InvoiceFormField id={id('company-address')} label={t('Company address')}>
        <Input
          id={id('company-address')}
          value={props.values.companyAddress}
          maxLength={512}
          onChange={(event) =>
            props.onChange('companyAddress', event.target.value)
          }
        />
      </InvoiceFormField>
      <InvoiceFormField id={id('company-phone')} label={t('Company phone')}>
        <Input
          id={id('company-phone')}
          value={props.values.companyPhone}
          maxLength={32}
          onChange={(event) =>
            props.onChange('companyPhone', event.target.value)
          }
        />
      </InvoiceFormField>
    </>
  )
}

type InvoiceFormFieldProps = {
  id: string
  label: string
  required?: boolean
  className?: string
  error?: string
  hint?: string
  hintTone?: 'muted' | 'warning'
  children: React.ReactNode
}

export function InvoiceFormField(props: InvoiceFormFieldProps) {
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

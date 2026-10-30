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
import type {
  InvoiceRegion,
  InvoiceRequest,
  InvoiceTitle,
  InvoiceTitlePayload,
} from '../types'
import { normalizeInvoiceTaxNumber } from './invoice-utils'

export const INVOICE_REGIONS: readonly InvoiceRegion[] = ['mainland', 'hk']

/** Translation keys naming each invoice type, e.g. in lists and copy text. */
export const INVOICE_REGION_LABEL_KEYS: Record<InvoiceRegion, string> = {
  mainland: 'VAT electronic ordinary invoice (Mainland China)',
  hk: 'INVOICE (Hong Kong)',
}

export function getInvoiceRegion(
  invoice: Pick<InvoiceRequest, 'region'>
): InvoiceRegion {
  return invoice.region === 'hk' ? 'hk' : 'mainland'
}

export type InvoiceTitleFormValues = {
  companyName: string
  taxNumber: string
  bankName: string
  bankAccount: string
  companyAddress: string
  companyPhone: string
}

export const EMPTY_INVOICE_TITLE_FORM: InvoiceTitleFormValues = {
  companyName: '',
  taxNumber: '',
  bankName: '',
  bankAccount: '',
  companyAddress: '',
  companyPhone: '',
}

export function invoiceTitleToFormValues(
  title: InvoiceTitle | undefined
): InvoiceTitleFormValues {
  if (!title) return EMPTY_INVOICE_TITLE_FORM
  return {
    companyName: title.company_name,
    taxNumber: title.tax_number,
    bankName: title.bank_name,
    bankAccount: title.bank_account,
    companyAddress: title.company_address,
    companyPhone: title.company_phone,
  }
}

/** The saved title marked as default for the region, if any. */
export function pickDefaultInvoiceTitle(
  titles: InvoiceTitle[],
  region: InvoiceRegion
): InvoiceTitle | undefined {
  return titles.find((title) => title.region === region && title.is_default)
}

/**
 * Billing details sent for a region. A Hong Kong invoice carries the
 * recipient name only, so mainland-only fields are always blank for it.
 */
export function buildInvoiceTitleFields(
  region: InvoiceRegion,
  values: InvoiceTitleFormValues
): Omit<InvoiceTitlePayload, 'region' | 'is_default'> {
  const companyName = values.companyName.trim()
  if (region === 'hk') {
    return {
      company_name: companyName,
      tax_number: '',
      bank_name: '',
      bank_account: '',
      company_address: '',
      company_phone: '',
    }
  }
  return {
    company_name: companyName,
    tax_number: normalizeInvoiceTaxNumber(values.taxNumber),
    bank_name: values.bankName.trim(),
    bank_account: values.bankAccount.trim(),
    company_address: values.companyAddress.trim(),
    company_phone: values.companyPhone.trim(),
  }
}

export function isInvoiceTitleComplete(
  region: InvoiceRegion,
  values: InvoiceTitleFormValues
): boolean {
  const fields = buildInvoiceTitleFields(region, values)
  if (fields.company_name === '') return false
  return region === 'hk' || fields.tax_number !== ''
}

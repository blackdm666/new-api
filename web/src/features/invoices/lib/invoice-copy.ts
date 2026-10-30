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
import type { InvoiceRequest } from '../types'
import { formatInvoiceAmount } from './invoice-utils'

export type InvoiceCopyLabels = {
  invoiceType: string
  /** The translated invoice type, e.g. "VAT electronic ordinary invoice (Mainland China)". */
  invoiceTypeValue: string
  companyName: string
  taxNumber: string
  bankName: string
  bankAccount: string
  companyAddress: string
  companyPhone: string
  remark: string
  totalAmount: string
}

function addLine(lines: string[], label: string, value: string) {
  const normalized = value.trim()
  if (normalized) lines.push(`${label}: ${normalized}`)
}

/**
 * Text pasted into the invoicing system: billing details and the invoice
 * amount only. Top-up orders are intentionally left out.
 */
export function formatInvoiceCopyText(
  invoice: InvoiceRequest,
  labels: InvoiceCopyLabels
): string {
  const lines: string[] = []
  // The invoice type comes first: it decides which system the details are
  // typed into.
  addLine(lines, labels.invoiceType, labels.invoiceTypeValue)
  addLine(lines, labels.companyName, invoice.company_name)
  addLine(lines, labels.taxNumber, invoice.tax_number)
  addLine(lines, labels.bankName, invoice.bank_name)
  addLine(lines, labels.bankAccount, invoice.bank_account)
  addLine(lines, labels.companyAddress, invoice.company_address)
  addLine(lines, labels.companyPhone, invoice.company_phone)
  addLine(lines, labels.remark, invoice.remark ?? '')
  lines.push(
    `${labels.totalAmount}: ${formatInvoiceAmount(invoice.total_money ?? 0)}`
  )
  return lines.join('\n')
}

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
import type { EligibleTopUpOrder, InvoiceRequest } from '../types'

export type InvoiceCopyLabels = {
  companyName: string
  taxNumber: string
  bankName: string
  bankAccount: string
  companyAddress: string
  companyPhone: string
  totalAmount: string
  includedOrders: string
  tradeNumber: string
  payment: string
  amount: string
}

function addLine(lines: string[], label: string, value: string) {
  const normalized = value.trim()
  if (normalized) lines.push(`${label}: ${normalized}`)
}

export function formatInvoiceCopyText(
  invoice: InvoiceRequest,
  orders: EligibleTopUpOrder[],
  labels: InvoiceCopyLabels
): string {
  const lines: string[] = []
  addLine(lines, labels.companyName, invoice.company_name)
  addLine(lines, labels.taxNumber, invoice.tax_number)
  addLine(lines, labels.bankName, invoice.bank_name)
  addLine(lines, labels.bankAccount, invoice.bank_account)
  addLine(lines, labels.companyAddress, invoice.company_address)
  addLine(lines, labels.companyPhone, invoice.company_phone)
  lines.push(`${labels.totalAmount}: ¥${(invoice.total_money ?? 0).toFixed(2)}`)
  if (orders.length > 0) {
    lines.push('', `${labels.includedOrders}:`)
    orders.forEach((order, index) => {
      const fields = [
        order.trade_no?.trim()
          ? `${labels.tradeNumber}: ${order.trade_no.trim()}`
          : '',
        order.payment_method?.trim()
          ? `${labels.payment}: ${order.payment_method.trim()}`
          : '',
        `${labels.amount}: ¥${(order.money ?? 0).toFixed(2)}`,
      ].filter(Boolean)
      lines.push(`${index + 1}. ${fields.join(' | ')}`)
    })
  }
  return lines.join('\n')
}

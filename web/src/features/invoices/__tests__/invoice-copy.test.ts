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
import { describe, expect, test } from 'vitest'

import { formatInvoiceCopyText } from '../lib/invoice-copy'
import { INVOICE_STATUS, type InvoiceRequest } from '../types'

const labels = {
  invoiceType: '开票类型',
  invoiceTypeValue: '增值税电子普通发票（中国大陆）',
  companyName: '发票抬头',
  taxNumber: '税号',
  bankName: '开户行',
  bankAccount: '银行账号',
  companyAddress: '公司地址',
  companyPhone: '公司电话',
  remark: '备注',
  totalAmount: '开票金额',
}

const invoice = {
  company_name: '上海示例科技有限公司',
  tax_number: '91310000TEST2026',
  bank_name: '招商银行上海分行',
  bank_account: '6225880000000000',
  company_address: '上海市浦东新区示例路 88 号',
  company_phone: '021-88888888',
  remark: '请备注项目名称',
  topup_order_ids: '1001,1002',
  total_money: 2979,
  status: INVOICE_STATUS.PENDING,
} as InvoiceRequest

describe('invoice information copy', () => {
  test('copies the billing details, remark and the formatted invoice amount', () => {
    const text = formatInvoiceCopyText(invoice, labels)

    expect(text.split('\n')).toEqual([
      '开票类型: 增值税电子普通发票（中国大陆）',
      '发票抬头: 上海示例科技有限公司',
      '税号: 91310000TEST2026',
      '开户行: 招商银行上海分行',
      '银行账号: 6225880000000000',
      '公司地址: 上海市浦东新区示例路 88 号',
      '公司电话: 021-88888888',
      '备注: 请备注项目名称',
      '开票金额: ¥2,979.00',
    ])
  })

  test('a Hong Kong invoice starts with its type and carries only the recipient name and amount', () => {
    const text = formatInvoiceCopyText(
      {
        ...invoice,
        region: 'hk',
        tax_number: '',
        bank_name: '',
        bank_account: '',
        company_address: '',
        company_phone: '',
        remark: '',
      },
      {
        ...labels,
        invoiceTypeValue: '帳單 INVOICE（中国香港）',
        companyName: '收件人名称',
      }
    )

    expect(text.split('\n')).toEqual([
      '开票类型: 帳單 INVOICE（中国香港）',
      '收件人名称: 上海示例科技有限公司',
      '开票金额: ¥2,979.00',
    ])
  })

  test('leaves out top-up orders and empty optional fields', () => {
    const text = formatInvoiceCopyText(
      { ...invoice, bank_name: '', bank_account: ' ', remark: '' },
      labels
    )

    expect(text).not.toContain('1001')
    expect(text).not.toContain('1002')
    expect(text).not.toContain('开户行')
    expect(text).not.toContain('银行账号')
    expect(text).not.toContain('备注')
    expect(text).toContain('开票金额: ¥2,979.00')
  })
})

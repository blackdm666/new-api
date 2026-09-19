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
  companyName: '公司名称',
  taxNumber: '税号',
  bankName: '开户行',
  bankAccount: '银行账号',
  companyAddress: '公司地址',
  companyPhone: '公司电话',
  totalAmount: '开票金额',
  includedOrders: '包含订单',
  tradeNumber: '订单号',
  payment: '支付方式',
  amount: '金额',
}

describe('invoice information copy', () => {
  test('includes billing fields and every claimed order', () => {
    const invoice = {
      company_name: '上海示例科技有限公司',
      tax_number: '91310000TEST2026',
      bank_name: '招商银行上海分行',
      bank_account: '6225880000000000',
      company_address: '上海市浦东新区示例路 88 号',
      company_phone: '021-88888888',
      total_money: 888,
      status: INVOICE_STATUS.PENDING,
    } as InvoiceRequest
    const text = formatInvoiceCopyText(
      invoice,
      [
        {
          id: 1,
          trade_no: 'PAY-20260814-001',
          payment_method: '支付宝',
          money: 888,
          amount: 888,
          status: 'success',
          create_time: 1,
        },
      ],
      labels
    )

    expect(text).toContain('公司名称: 上海示例科技有限公司')
    expect(text).toContain('税号: 91310000TEST2026')
    expect(text).toContain('订单号: PAY-20260814-001')
    expect(text).toContain('开票金额: ¥888.00')
  })
})

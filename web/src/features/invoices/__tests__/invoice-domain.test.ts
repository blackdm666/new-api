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

import { buildInvoiceListQuery } from '../api'
import { getInvoiceAmountShortfall } from '../lib/invoice-utils'
import { INVOICE_STATUS, INVOICE_STATUS_OPTIONS } from '../types'

describe('independent invoice domain', () => {
  test('uses the invoice lifecycle statuses directly', () => {
    expect(INVOICE_STATUS_OPTIONS).toEqual([
      INVOICE_STATUS.PENDING,
      INVOICE_STATUS.ISSUED,
      INVOICE_STATUS.REJECTED,
      INVOICE_STATUS.WITHDRAWN,
      INVOICE_STATUS.EXPIRED,
    ])
  })

  test('builds an invoice-only status query', () => {
    const query = new URLSearchParams(
      buildInvoiceListQuery({
        page: 1,
        pageSize: 20,
        status: INVOICE_STATUS.ISSUED,
        keyword: '91310000INVOICE',
      })
    )

    expect(query.get('status')).toBe(String(INVOICE_STATUS.ISSUED))
    expect(query.get('keyword')).toBe('91310000INVOICE')
    expect([...query.keys()]).toEqual(['p', 'page_size', 'status', 'keyword'])
  })

  test('accepts exactly the minimum invoice amount and reports the shortfall below it', () => {
    expect(getInvoiceAmountShortfall(500, 500)).toBe(0)
    expect(getInvoiceAmountShortfall(499.99, 500)).toBeCloseTo(0.01, 8)
  })
})

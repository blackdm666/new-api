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

import {
  buildInvoiceListSearch,
  invoiceListSearchSchema,
  resolveInvoiceListState,
} from '../lib/invoice-list-search'
import { INVOICE_STATUS } from '../types'

describe('invoice list search params', () => {
  test('administrators see pending applications when the URL has no status', () => {
    const search = invoiceListSearchSchema.parse({})

    expect(resolveInvoiceListState(search, true).status).toBe(
      INVOICE_STATUS.PENDING
    )
    expect(resolveInvoiceListState(search, false).status).toBe('all')
  })

  test('an explicit all-status filter overrides the administrator default', () => {
    const search = invoiceListSearchSchema.parse({ status: 'all', page: 3 })

    expect(resolveInvoiceListState(search, true)).toEqual({
      page: 3,
      pageSize: 10,
      status: 'all',
      keyword: '',
    })
  })

  test('invalid or hand-edited values fall back to defaults instead of failing', () => {
    const search = invoiceListSearchSchema.parse({
      page: -2,
      pageSize: 33,
      status: 99,
      keyword: 91310000123456,
    })

    expect(resolveInvoiceListState(search, true)).toEqual({
      page: 1,
      pageSize: 10,
      status: INVOICE_STATUS.PENDING,
      keyword: '91310000123456',
    })
  })

  test('building search params omits defaults and keeps filters', () => {
    expect(
      buildInvoiceListSearch(
        {
          page: 1,
          pageSize: 10,
          status: INVOICE_STATUS.PENDING,
          keyword: '  ',
        },
        true
      )
    ).toEqual({
      page: undefined,
      pageSize: undefined,
      status: undefined,
      keyword: undefined,
    })
    expect(
      buildInvoiceListSearch(
        {
          page: 2,
          pageSize: 50,
          status: INVOICE_STATUS.REJECTED,
          keyword: 'PAY-001',
        },
        true
      )
    ).toEqual({
      page: 2,
      pageSize: 50,
      status: INVOICE_STATUS.REJECTED,
      keyword: 'PAY-001',
    })
  })
})

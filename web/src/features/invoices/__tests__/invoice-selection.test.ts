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
// @ts-expect-error The CI runtime provides bun:test; the application tsconfig intentionally omits Bun globals.
import { describe, expect, test } from 'bun:test'

import {
  MAX_INVOICE_ORDER_SELECTION,
  selectInvoiceOrderIds,
} from '../lib/invoice-selection'

describe('invoice order selection', () => {
  test('select all never selects more than the backend limit', () => {
    const ids = Array.from({ length: 150 }, (_, index) => index + 1)

    const selected = selectInvoiceOrderIds(ids)

    expect(selected.size).toBe(MAX_INVOICE_ORDER_SELECTION)
    expect(selected.has(100)).toBe(true)
    expect(selected.has(101)).toBe(false)
  })
})

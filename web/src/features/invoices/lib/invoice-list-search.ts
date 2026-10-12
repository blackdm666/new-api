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
import { z } from 'zod'

import { INVOICE_STATUS, type InvoiceStatus } from '../types'

export const INVOICE_PAGE_SIZES = [10, 20, 50, 100] as const
const DEFAULT_PAGE_SIZE = 10

/**
 * URL search params of the invoice list pages. Invalid values are dropped
 * instead of failing the route so stale or hand-edited links still open.
 */
export const invoiceListSearchSchema = z.object({
  page: z.number().int().min(1).optional().catch(undefined),
  pageSize: z
    .union([z.literal(10), z.literal(20), z.literal(50), z.literal(100)])
    .optional()
    .catch(undefined),
  status: z
    .union([
      z.literal('all'),
      z.literal(INVOICE_STATUS.PENDING),
      z.literal(INVOICE_STATUS.ISSUED),
      z.literal(INVOICE_STATUS.REJECTED),
      z.literal(INVOICE_STATUS.WITHDRAWN),
      z.literal(INVOICE_STATUS.EXPIRED),
    ])
    .optional()
    .catch(undefined),
  // Numeric-looking keywords typed into the address bar arrive as numbers.
  keyword: z
    .union([z.string(), z.number()])
    .transform((value) => String(value).trim() || undefined)
    .optional()
    .catch(undefined),
})

export type InvoiceListSearch = z.output<typeof invoiceListSearchSchema>
export type InvoiceStatusFilter = 'all' | InvoiceStatus

export type InvoiceListState = {
  page: number
  pageSize: number
  status: InvoiceStatusFilter
  keyword: string
}

/** Administrators work through the pending queue by default. */
export function getDefaultInvoiceStatusFilter(
  admin: boolean
): InvoiceStatusFilter {
  return admin ? INVOICE_STATUS.PENDING : 'all'
}

export function resolveInvoiceListState(
  search: InvoiceListSearch,
  admin: boolean
): InvoiceListState {
  return {
    page: search.page ?? 1,
    pageSize: search.pageSize ?? DEFAULT_PAGE_SIZE,
    status: search.status ?? getDefaultInvoiceStatusFilter(admin),
    keyword: search.keyword ?? '',
  }
}

/**
 * Builds the next search params from the effective list state, omitting
 * defaults so the URL stays short.
 */
export function buildInvoiceListSearch(
  state: InvoiceListState,
  admin: boolean
): InvoiceListSearch {
  const pageSize =
    state.pageSize === DEFAULT_PAGE_SIZE
      ? undefined
      : INVOICE_PAGE_SIZES.find((size) => size === state.pageSize)
  return {
    page: state.page > 1 ? state.page : undefined,
    pageSize,
    status:
      state.status === getDefaultInvoiceStatusFilter(admin)
        ? undefined
        : state.status,
    keyword: state.keyword.trim() || undefined,
  }
}

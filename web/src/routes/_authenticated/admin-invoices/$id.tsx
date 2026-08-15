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
import { createFileRoute, redirect, useParams } from '@tanstack/react-router'

import { InvoiceDetailPage } from '@/features/invoices/components/invoice-detail-page'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

function AdminInvoiceDetailRoute() {
  const { id } = useParams({ from: '/_authenticated/admin-invoices/$id' })
  const invoiceId = Number(id)
  return (
    <InvoiceDetailPage
      invoiceId={Number.isFinite(invoiceId) ? invoiceId : 0}
      admin
    />
  )
}

export const Route = createFileRoute('/_authenticated/admin-invoices/$id')({
  beforeLoad: () => {
    const { auth } = useAuthStore.getState()
    if (!auth.user || (auth.user.role ?? 0) < ROLE.ADMIN) {
      throw redirect({ to: '/403' })
    }
  },
  component: AdminInvoiceDetailRoute,
})

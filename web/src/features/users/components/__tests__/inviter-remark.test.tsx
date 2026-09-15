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
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { cleanup, render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, expect, test } from 'vitest'

import type { User } from '../../types'
import { useUsersColumns } from '../users-columns'

afterEach(cleanup)

const user: User = {
  id: 2691,
  username: 'invitee',
  display_name: '',
  role: 1,
  status: 1,
  quota: 0,
  used_quota: 0,
  request_count: 0,
  group: 'default',
  inviter_id: 1726,
  inviter_remark: 'Deepseek桌面版',
  aff_count: 777,
  affiliate_lifetime_earned_cents: 1350,
}

function InviteCell({ value }: { value: User }) {
  const columns = useUsersColumns()
  const table = useReactTable({
    data: [value],
    columns,
    getCoreRowModel: getCoreRowModel(),
  })
  const cell = table
    .getRowModel()
    .rows[0].getVisibleCells()
    .find((item) => item.column.id === 'invite_info')
  if (!cell) throw new Error('Invitation column was not found')
  return <>{flexRender(cell.column.columnDef.cell, cell.getContext())}</>
}

async function renderCell(value: User) {
  const i18n = createInstance()
  await i18n
    .use(initReactI18next)
    .init({ lng: 'en', resources: { en: { translation: {} } } })
  return render(
    <I18nextProvider i18n={i18n}>
      <InviteCell value={value} />
    </I18nextProvider>
  )
}

test('shows inviter ID and remark while retaining earnings and hiding obsolete invite count', async () => {
  await renderCell(user)
  expect(screen.getByText('Inviter: 1726')).toBeVisible()
  expect(screen.getByText('Remark: Deepseek桌面版')).toBeVisible()
  expect(screen.getByText(/^Revenue:/)).toBeVisible()
  expect(screen.queryByText(/^Invited:/)).not.toBeInTheDocument()
  expect(screen.queryByText(/777/)).not.toBeInTheDocument()
})

test('does not show an empty remark label or a stale remark when no inviter exists', async () => {
  await renderCell({ ...user, inviter_id: 0 })
  expect(screen.getByText('No Inviter')).toBeVisible()
  expect(screen.queryByText(/^Remark:/)).not.toBeInTheDocument()
})

test('renders long administrator remarks as plain text', async () => {
  const remark = '<img src=x onerror=alert(1)> '.repeat(12).trim()
  const { container } = await renderCell({ ...user, inviter_remark: remark })
  expect(screen.getByText(`Remark: ${remark}`)).toBeVisible()
  expect(container.querySelector('img')).toBeNull()
})

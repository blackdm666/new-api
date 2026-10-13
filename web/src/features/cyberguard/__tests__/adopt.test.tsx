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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { AdoptForm } from '../components/adopt-form'
import type { CyberGuardAdoptableUser } from '../types'

const BAN_GROUP = '您因违反使用规定已被封禁'

// The envelope every other screen caches under the shared ['groups'] key.
const groupsEnvelope = {
  success: true,
  message: '',
  data: ['default', '代理商', BAN_GROUP],
}

const reseller: CyberGuardAdoptableUser = {
  user_id: 2752,
  username: 'user2752',
  remark: '原分组:代理商',
  suggested_original_group: '代理商',
}

function renderAdoptForm(client: QueryClient) {
  render(
    <QueryClientProvider client={client}>
      <AdoptForm
        user={reseller}
        banGroup={BAN_GROUP}
        onDone={() => undefined}
        onCancel={() => undefined}
      />
    </QueryClientProvider>
  )
}

function groupOptionLabels() {
  const select = screen.getByLabelText('Group to restore on unban')
  return within(select)
    .getAllByRole('option')
    .map((option) => option.textContent)
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('import form group picker', () => {
  test('groups cached by another screen as the API envelope render as options', async () => {
    vi.spyOn(api, 'get').mockRejectedValue(new Error('unexpected request'))
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Infinity } },
    })
    client.setQueryData(['groups'], {
      success: true,
      message: '',
      data: ['default', '代理商'],
    })

    renderAdoptForm(client)

    expect(await screen.findByRole('option', { name: 'default' })).toBeVisible()
    expect(groupOptionLabels()).toEqual(['代理商', 'default'])
    expect(screen.getByLabelText('Group to restore on unban')).toHaveValue(
      '代理商'
    )
  })

  test('fetched groups stay cached as the envelope other screens read and exclude the ban group', async () => {
    const get = vi.spyOn(api, 'get').mockImplementation((url) => {
      if (url === '/api/group/') {
        return Promise.resolve({ data: groupsEnvelope })
      }
      return Promise.reject(new Error(`unexpected GET ${url}`))
    })
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })

    renderAdoptForm(client)

    expect(await screen.findByRole('option', { name: 'default' })).toBeVisible()
    expect(get).toHaveBeenCalledWith('/api/group/')
    expect(groupOptionLabels()).toEqual(['代理商', 'default'])
    await waitFor(() =>
      expect(client.getQueryData(['groups'])).toEqual(groupsEnvelope)
    )
  })
})

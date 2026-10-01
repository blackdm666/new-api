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
import type { AxiosAdapter, InternalAxiosRequestConfig } from 'axios'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { downloadInvoiceFile } from '../api'

const invoiceFile = {
  id: 7,
  invoice_request_id: 42,
  uploader_id: 1,
  file_name: 'invoice.pdf',
  mime_type: 'application/pdf',
  size: 3,
  storage_type: 'local',
  sha256: 'hash',
  created_time: 1,
}

describe('invoice file download', () => {
  const originalAdapter = api.defaults.adapter

  afterEach(() => {
    api.defaults.adapter = originalAdapter
    useAuthStore.getState().auth.reset()
    vi.restoreAllMocks()
  })

  test('fetches the protected endpoint with the shared authenticated client', async () => {
    useAuthStore.getState().auth.setBundle({
      access_token: 'invoice-access-token',
      token_type: 'Bearer',
      access_expires_at: 2_000_000_000,
      user: { id: 1, username: 'invoice-user', role: 1 },
      session: {
        sid: 'invoice-session',
        current: true,
        login_method: 'password',
        ip: '127.0.0.1',
        user_agent: 'vitest',
        created_at: 1,
        last_active_at: 1,
        expires_at: 2_000_000_000,
      },
    })
    let captured: InternalAxiosRequestConfig | undefined
    const adapter: AxiosAdapter = async (config) => {
      captured = config
      return {
        config,
        data: new Blob(['pdf'], { type: 'application/pdf' }),
        headers: {},
        status: 200,
        statusText: 'OK',
      }
    }
    api.defaults.adapter = adapter

    const objectUrl = 'blob:test-invoice'
    vi.spyOn(URL, 'createObjectURL').mockReturnValue(objectUrl)
    vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => undefined)
    const click = vi
      .spyOn(HTMLAnchorElement.prototype, 'click')
      .mockImplementation(() => undefined)

    await downloadInvoiceFile(42, invoiceFile)

    expect(captured?.url).toBe('/api/invoice/requests/42/files/7')
    expect(captured?.responseType).toBe('blob')
    expect(captured?.headers.get('Authorization')).toBe(
      'Bearer invoice-access-token'
    )
    expect(click).toHaveBeenCalledOnce()
    expect(URL.createObjectURL).toHaveBeenCalledOnce()
    expect(URL.revokeObjectURL).not.toHaveBeenCalled()

    await new Promise((resolve) => window.setTimeout(resolve, 0))
    expect(URL.revokeObjectURL).toHaveBeenCalledWith(objectUrl)
  })
})

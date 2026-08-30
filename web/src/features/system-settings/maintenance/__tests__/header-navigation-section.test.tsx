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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { HEADER_NAV_DEFAULT, serializeHeaderNavModules } from '../config'
import { HeaderNavigationSection } from '../header-navigation-section'

const mutateAsync = vi.fn()

vi.mock('../../hooks/use-update-option', () => ({
  useUpdateOption: () => ({ mutateAsync, isPending: false }),
}))

vi.mock('../../components/settings-page-context', async (importOriginal) => ({
  ...(await importOriginal<
    typeof import('../../components/settings-page-context')
  >()),
  SettingsPageFormActions: (props: { onSave: () => void }) => (
    <button type='button' onClick={() => props.onSave()}>
      Save navigation
    </button>
  ),
}))

describe('header navigation Infinite Canvas settings', () => {
  afterEach(() => {
    mutateAsync.mockReset()
  })

  test('edits and saves the button name and URL in HeaderNavModules', async () => {
    render(
      <HeaderNavigationSection
        config={HEADER_NAV_DEFAULT}
        initialSerialized={serializeHeaderNavModules(HEADER_NAV_DEFAULT)}
      />
    )

    const nameInput = screen.getByRole('textbox', { name: 'Button name' })
    const urlInput = screen.getByRole('textbox', { name: 'Button URL' })
    expect(nameInput).toHaveValue('Infinite Canvas')
    expect(urlInput).toHaveValue('https://img-pro.88api.ai')

    fireEvent.change(nameInput, { target: { value: '创作工作台' } })
    fireEvent.change(urlInput, {
      target: { value: 'https://canvas.example.com/workspace' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save navigation' }))

    await waitFor(() => expect(mutateAsync).toHaveBeenCalledTimes(1))
    const request = mutateAsync.mock.calls[0]?.[0] as {
      key: string
      value: string
    }
    expect(request.key).toBe('HeaderNavModules')
    expect(JSON.parse(request.value).infiniteCanvas).toEqual({
      name: '创作工作台',
      url: 'https://canvas.example.com/workspace',
    })
  })

  test('rejects a non-HTTP destination before saving', async () => {
    render(
      <HeaderNavigationSection
        config={HEADER_NAV_DEFAULT}
        initialSerialized={serializeHeaderNavModules(HEADER_NAV_DEFAULT)}
      />
    )

    fireEvent.change(screen.getByRole('textbox', { name: 'Button URL' }), {
      target: { value: 'javascript:alert(1)' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save navigation' }))

    expect(await screen.findByText('Must be a valid URL')).toBeInTheDocument()
    expect(mutateAsync).not.toHaveBeenCalled()
  })
})

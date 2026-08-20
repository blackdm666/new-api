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
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, test, vi } from 'vitest'

const i18n = (await import('i18next')).default
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { RedemptionUserLink } = await import('../redemption-user-link')

await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

function renderLink(
  userId: number,
  onUserClick: (userId: number) => void,
  onContainerClick = () => undefined
) {
  return render(
    <I18nextProvider i18n={i18n}>
      <div onClick={onContainerClick}>
        <RedemptionUserLink
          userId={userId}
          redeemedTime={1_786_700_200}
          onUserClick={onUserClick}
        />
      </div>
    </I18nextProvider>
  )
}

describe('redemption code redeemed user link', () => {
  test('opens user information for the redeemed user without triggering its row', () => {
    const onUserClick = vi.fn()
    const onContainerClick = vi.fn()
    renderLink(1387, onUserClick, onContainerClick)

    fireEvent.click(
      screen.getByRole('button', {
        name: 'User Information: User 1387',
      })
    )

    expect(onUserClick).toHaveBeenCalledOnce()
    expect(onUserClick).toHaveBeenCalledWith(1387)
    expect(onContainerClick).not.toHaveBeenCalled()
  })

  test('keeps unused redemption codes as a non-interactive dash', () => {
    renderLink(0, vi.fn())

    expect(screen.getByText('-')).toBeVisible()
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })
})

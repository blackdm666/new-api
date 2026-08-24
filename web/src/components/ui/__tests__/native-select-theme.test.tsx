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
import { render, screen } from '@testing-library/react'
import { describe, expect, test } from 'vitest'

import { NativeSelect } from '../native-select'

describe('native select theme', () => {
  test('uses the active theme for the browser-native popup and options', () => {
    render(
      <NativeSelect aria-label='Language'>
        <option value='zh-CN'>简体中文</option>
        <option value='en'>English</option>
      </NativeSelect>
    )

    const select = screen.getByRole('combobox', { name: 'Language' })
    expect(select).toHaveClass('[color-scheme:light]')
    expect(select).toHaveClass('dark:[color-scheme:dark]')
    expect(select).toHaveClass('[&>option]:bg-popover')
    expect(select).toHaveClass('[&>option]:text-popover-foreground')
  })
})

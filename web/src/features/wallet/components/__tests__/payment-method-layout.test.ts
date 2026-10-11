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
import assert from 'node:assert/strict'

import { describe, test } from 'vitest'

import {
  paymentMethodGridClassName,
  paymentMethodSectionClassName,
} from '../payment-method-layout'

describe('Payment method button layout', () => {
  test('uses two columns by default and four columns once the section is wide', () => {
    const classes = new Set(paymentMethodGridClassName.split(' '))

    assert.ok(classes.has('grid'))
    assert.ok(classes.has('grid-cols-2'))
    assert.ok(classes.has('@2xl/payment-methods:grid-cols-4'))
    assert.ok(!classes.has('lg:grid-cols-3'))
  })

  test('declares the named container the grid breakpoint queries', () => {
    const classes = new Set(paymentMethodSectionClassName.split(' '))

    assert.ok(classes.has('@container/payment-methods'))
  })
})

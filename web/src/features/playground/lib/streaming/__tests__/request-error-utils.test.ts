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
import { describe, expect, test } from 'vitest'

import {
  getPlaygroundRequestErrorMessage,
  parseRequestErrorDetails,
} from '../request-error-utils'

describe('parseRequestErrorDetails', () => {
  test('preserves an OpenAI-format insufficient balance error', () => {
    expect(
      parseRequestErrorDetails({
        response: {
          data: {
            error: {
              code: 'insufficient_user_quota',
              message: '用户额度不足, 剩余额度: $0.00',
            },
          },
        },
      })
    ).toEqual({
      errorCode: 'insufficient_user_quota',
      errorMessage: '用户额度不足, 剩余额度: $0.00',
    })
  })

  test('preserves a task-format insufficient balance error', () => {
    expect(
      parseRequestErrorDetails({
        response: {
          data: {
            code: 'insufficient_user_quota',
            message: '用户额度不足, 剩余额度: $0.00',
          },
        },
      })
    ).toEqual({
      errorCode: 'insufficient_user_quota',
      errorMessage: '用户额度不足, 剩余额度: $0.00',
    })
  })

  test('renders insufficient quota as the localized balance error', () => {
    expect(
      getPlaygroundRequestErrorMessage(
        {
          errorCode: 'insufficient_user_quota',
          errorMessage: 'a generic upstream error',
        },
        (key) => (key === 'Insufficient balance' ? '余额不足' : key)
      )
    ).toBe('余额不足')
  })
})

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
import { ERROR_MESSAGES } from '../../constants'

type RequestErrorLike = {
  message?: string
  response?: {
    data?: {
      code?: string
      error?: {
        code?: string
        message?: string
      }
      message?: string
    }
  }
}

export type RequestErrorDetails = {
  errorCode?: string
  errorMessage: string
}

const INSUFFICIENT_QUOTA_CODE = 'insufficient_user_quota'

export function getPlaygroundRequestErrorMessage(
  details: RequestErrorDetails,
  translate: (key: string) => string
) {
  return details.errorCode === INSUFFICIENT_QUOTA_CODE
    ? translate('Insufficient balance')
    : details.errorMessage
}

export function parseRequestErrorDetails(error: unknown): RequestErrorDetails {
  const requestError = error as RequestErrorLike

  return {
    errorCode:
      requestError?.response?.data?.error?.code ||
      requestError?.response?.data?.code ||
      undefined,
    errorMessage:
      requestError?.response?.data?.error?.message ||
      requestError?.response?.data?.message ||
      requestError?.message ||
      ERROR_MESSAGES.API_REQUEST_ERROR,
  }
}

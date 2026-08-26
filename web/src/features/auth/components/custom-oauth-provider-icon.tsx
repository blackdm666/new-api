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
import type { ReactNode } from 'react'

import {
  resolveCustomOAuthBrand,
  type CustomOAuthProviderIdentity,
} from './custom-oauth-provider-brand'

type CustomOAuthProviderIconProps = {
  provider: CustomOAuthProviderIdentity
  className?: string
  fallback?: ReactNode
}

export function CustomOAuthProviderIcon({
  provider,
  className = 'h-4 w-4',
  fallback = null,
}: CustomOAuthProviderIconProps) {
  const brand = resolveCustomOAuthBrand(provider)

  if (brand === 'google') {
    return (
      <img
        src='/google-g-logo.svg'
        alt=''
        aria-hidden='true'
        className={className}
      />
    )
  }

  if (brand === 'microsoft') {
    return (
      <img
        src='/microsoft-logo.svg'
        alt=''
        aria-hidden='true'
        className={className}
      />
    )
  }

  return fallback
}

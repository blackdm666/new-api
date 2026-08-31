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
import i18next from 'i18next'
import { useState } from 'react'
import { toast } from 'sonner'

import {
  getTurnstileClientConfig,
  isTurnstileClientConfigured,
} from '@/components/turnstile-utils'
import { useStatus } from '@/hooks/use-status'

/**
 * Hook for managing Turnstile verification
 */
export function useTurnstile() {
  const { status, loading: isStatusLoading } = useStatus()
  const [turnstileToken, setTurnstileToken] = useState('')

  const turnstileConfig = getTurnstileClientConfig(status)
  const isTurnstileEnabled = Boolean(
    status?.turnstile_check && isTurnstileClientConfigured(turnstileConfig)
  )
  const isStatusReady = !isStatusLoading && Boolean(status)

  /**
   * Validate if turnstile is ready when required
   */
  const validateTurnstile = (): boolean => {
    if (!isStatusReady) {
      toast.info(
        i18next.t('Please wait a moment, human check is initializing...')
      )
      return false
    }
    if (isTurnstileEnabled && !turnstileToken) {
      toast.info(
        i18next.t('Please wait a moment, human check is initializing...')
      )
      return false
    }
    return true
  }

  return {
    isTurnstileEnabled,
    turnstileConfig,
    turnstileToken,
    setTurnstileToken,
    validateTurnstile,
    isStatusReady,
  }
}

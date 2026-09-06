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
import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { useSecureVerification } from '@/features/auth/secure-verification'
import { AuthOperationError } from '@/lib/secure-verification'

import { getChannelKey, getChannelBalanceQueryToken } from '../api'

export function useChannelKeyDisclosure(
  open: boolean,
  channelId: number | null,
  kind: 'key' | 'balance-query-token' = 'key'
) {
  const { t } = useTranslation()
  const verification = useSecureVerification()
  const cancelVerification = verification.cancel
  const requestVerification = verification.requestVerification
  const [disclosedKey, setDisclosedKey] = useState<{
    channelId: number
    key: string
  } | null>(null)
  const [isChannelKeyLoading, setIsChannelKeyLoading] = useState(false)
  const operation = useRef<AbortController | null>(null)

  const clearDisclosure = useCallback(() => {
    operation.current?.abort()
    operation.current = null
    cancelVerification()
    setDisclosedKey(null)
    setIsChannelKeyLoading(false)
  }, [cancelVerification])

  useEffect(() => {
    setDisclosedKey(null)
    setIsChannelKeyLoading(false)
    return () => {
      operation.current?.abort()
      operation.current = null
      cancelVerification()
    }
  }, [open, channelId, kind, cancelVerification])

  const handleRevealKey = useCallback(async () => {
    if (!channelId || !open || operation.current) return
    const current = new AbortController()
    operation.current = current
    try {
      const proof = await requestVerification({
        scope:
          kind === 'key' ? 'channel.key.read' : 'channel.balance_token.read',
        context: { channel_id: channelId },
        title: t('Verify to view channel key'),
        description: t(
          'Use Passkey or 2FA to confirm your identity before revealing this channel key.'
        ),
      })
      if (!proof || operation.current !== current) return
      setIsChannelKeyLoading(true)
      const res =
        kind === 'key'
          ? await getChannelKey(channelId, proof.proof_token, current.signal)
          : await getChannelBalanceQueryToken(
              channelId,
              proof.proof_token,
              current.signal
            )
      if (operation.current !== current) return
      if (!res.success) {
        throw new Error(res.message || t('Failed to fetch channel key'))
      }
      let key = ''
      if (res.data && 'key' in res.data) key = res.data.key
      if (res.data && 'token' in res.data) key = res.data.token
      setDisclosedKey({ channelId, key })
      toast.success(
        t(
          kind === 'key'
            ? 'Channel key unlocked'
            : 'Balance query token unlocked'
        )
      )
      return key
    } catch (error) {
      if (operation.current === current && !current.signal.aborted) {
        toast.error(t(AuthOperationError.from(error).message))
      }
    } finally {
      if (operation.current === current) {
        operation.current = null
        setIsChannelKeyLoading(false)
      }
    }
  }, [channelId, kind, open, requestVerification, t])

  const channelKey =
    open && disclosedKey?.channelId === channelId ? disclosedKey.key : null
  return {
    channelKey,
    isChannelKeyLoading,
    handleRevealKey,
    verification,
    clearDisclosure,
  }
}

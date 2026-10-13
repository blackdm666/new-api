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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { handleServerError } from '@/lib/handle-server-error'

import { CYBERGUARD_QUERY_KEY, voidCyberGuardEvent } from '../api'
import type { CyberGuardEvent } from '../types'

/** Voids one event (reason required); voided events never count. */
export function VoidEventDialog(props: {
  event: CyberGuardEvent | null
  onClose: () => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [reason, setReason] = useState('')
  const mutation = useMutation({
    mutationFn: (target: { id: number; reason: string }) =>
      voidCyberGuardEvent(target.id, target.reason),
    onSuccess: async () => {
      toast.success(t('Event voided.'))
      setReason('')
      props.onClose()
      await queryClient.invalidateQueries({ queryKey: CYBERGUARD_QUERY_KEY })
    },
    onError: (error) => handleServerError(error, t('The action failed.')),
  })
  const trimmed = reason.trim()

  return (
    <ConfirmDialog
      open={props.event !== null}
      onOpenChange={(open) => {
        if (open || mutation.isPending) return
        setReason('')
        props.onClose()
      }}
      title={t('Void this event')}
      desc={t(
        'A voided event never counts toward a ban. An open proposal it caused stays open for you to dismiss.'
      )}
      confirmText={t('Void')}
      destructive
      disabled={trimmed === ''}
      isLoading={mutation.isPending}
      handleConfirm={() => {
        if (props.event && trimmed && !mutation.isPending) {
          mutation.mutate({ id: props.event.id, reason: trimmed })
        }
      }}
    >
      <div className='grid gap-2'>
        <Label htmlFor='cyberguard-void-reason'>{t('Reason')}</Label>
        <Textarea
          id='cyberguard-void-reason'
          value={reason}
          maxLength={255}
          rows={3}
          required
          placeholder={t('For example: false positive confirmed with the user')}
          onChange={(event) => setReason(event.target.value)}
          disabled={mutation.isPending}
        />
      </div>
    </ConfirmDialog>
  )
}

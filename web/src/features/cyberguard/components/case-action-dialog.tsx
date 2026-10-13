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
import type { TFunction } from 'i18next'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { handleServerError } from '@/lib/handle-server-error'

import {
  CYBERGUARD_QUERY_KEY,
  dismissCyberGuardCase,
  promoteCyberGuardCase,
  resendCyberGuardBanEmail,
  unbanCyberGuardCase,
} from '../api'
import { emailStateLabel } from '../lib/labels'

export type CaseAction = {
  kind: 'unban' | 'promote' | 'dismiss' | 'resend'
  banId: number
  caseId: string
  username: string
}

type Outcome = { message: string; warning?: boolean }

async function runCaseAction(
  action: CaseAction,
  note: string,
  t: TFunction
): Promise<Outcome> {
  const caseId = action.caseId
  if (action.kind === 'promote') {
    const ban = await promoteCyberGuardCase(action.banId)
    return { message: t('Ban {{caseId}} applied.', { caseId: ban.case_id }) }
  }
  if (action.kind === 'dismiss') {
    await dismissCyberGuardCase(action.banId, note)
    return { message: t('Proposal {{caseId}} dismissed.', { caseId }) }
  }
  if (action.kind === 'resend') {
    const ban = await resendCyberGuardBanEmail(action.banId)
    if (ban.email_state === 'queued') {
      return { message: t('The ban email was queued again.') }
    }
    const state = emailStateLabel(ban.email_state, '', t)
    return {
      message: t('The ban email was not queued: {{state}}', { state }),
      warning: true,
    }
  }
  const result = await unbanCyberGuardCase(action.banId, note)
  if (result.group_changed) {
    return {
      message: t('{{caseId}} ended. Group restored to {{group}}.', {
        caseId,
        group: result.to_group,
      }),
    }
  }
  if (result.ban.end_reason === 'user_missing') {
    return {
      message: t('{{caseId}} ended. The user no longer exists.', { caseId }),
    }
  }
  return {
    message: t(
      '{{caseId}} ended. An administrator had already changed the group, so it was left as is.',
      { caseId }
    ),
    warning: true,
  }
}

/** Confirmation for unban / promote / dismiss / resend on one case. */
export function CaseActionDialog(props: {
  action: CaseAction | null
  onClose: () => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [note, setNote] = useState('')
  const action = props.action
  const mutation = useMutation({
    mutationFn: (target: CaseAction) => runCaseAction(target, note.trim(), t),
    onSuccess: async (outcome) => {
      if (outcome.warning) toast.warning(outcome.message)
      else toast.success(outcome.message)
      setNote('')
      props.onClose()
      await queryClient.invalidateQueries({ queryKey: CYBERGUARD_QUERY_KEY })
    },
    onError: (error) => handleServerError(error, t('The action failed.')),
  })
  const takesNote = action?.kind === 'unban' || action?.kind === 'dismiss'
  const copy = {
    unban: {
      title: t('Unban {{caseId}}', { caseId: action?.caseId }),
      desc: t(
        '{{username}} is moved back to the group they had before the ban. No email is sent.',
        { username: action?.username }
      ),
      confirm: t('Unban'),
    },
    promote: {
      title: t('Apply the ban proposed in {{caseId}}', {
        caseId: action?.caseId,
      }),
      desc: t(
        '{{username}} is moved into the ban group and, if ban emails are on, receives the ban email. This works in every mode.',
        { username: action?.username }
      ),
      confirm: t('Apply ban'),
    },
    dismiss: {
      title: t('Dismiss proposal {{caseId}}', { caseId: action?.caseId }),
      desc: t(
        'The proposal is closed without a ban. Sessions flagged before now no longer count toward a tier.'
      ),
      confirm: t('Dismiss'),
    },
    resend: {
      title: t('Resend the ban email of {{caseId}}', {
        caseId: action?.caseId,
      }),
      desc: t(
        'A new ban email is queued for {{username}}, even when ban emails are turned off in settings.',
        { username: action?.username }
      ),
      confirm: t('Resend email'),
    },
  }[action?.kind ?? 'unban']

  return (
    <ConfirmDialog
      open={action !== null}
      onOpenChange={(open) => {
        if (open || mutation.isPending) return
        setNote('')
        props.onClose()
      }}
      title={copy.title}
      desc={copy.desc}
      confirmText={copy.confirm}
      destructive={action?.kind === 'promote'}
      isLoading={mutation.isPending}
      handleConfirm={() => {
        if (action && !mutation.isPending) mutation.mutate(action)
      }}
    >
      {takesNote && (
        <div className='grid gap-2'>
          <Label htmlFor='cyberguard-case-note'>{t('Note (optional)')}</Label>
          <Textarea
            id='cyberguard-case-note'
            value={note}
            maxLength={255}
            rows={3}
            onChange={(event) => setNote(event.target.value)}
            disabled={mutation.isPending}
          />
        </div>
      )}
    </ConfirmDialog>
  )
}

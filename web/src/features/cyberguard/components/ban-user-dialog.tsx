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
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { GroupBadge } from '@/components/group-badge'
import { Button } from '@/components/ui/button'
import { handleServerError } from '@/lib/handle-server-error'

import { CYBERGUARD_QUERY_KEY, createCyberGuardBan } from '../api'
import { getBanFormSchema, type BanFormValues } from '../lib/forms'
import type { CyberGuardUserRow } from '../types'
import { BanTermsFields } from './ban-terms-fields'

const FORM_ID = 'cyberguard-ban-user-form'

/** Manual ban of one user; works in every mode. */
export function BanUserDialog(props: {
  user: CyberGuardUserRow | null
  onClose: () => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const form = useForm<BanFormValues>({
    resolver: zodResolver(getBanFormSchema(t)),
    defaultValues: { hours: '72', reason: '' },
  })
  const mutation = useMutation({
    mutationFn: (values: BanFormValues & { userId: number }) =>
      createCyberGuardBan({
        user_id: values.userId,
        hours: Number(values.hours),
        reason: values.reason,
      }),
    onSuccess: async (ban) => {
      toast.success(
        t('{{username}} is banned ({{caseId}}).', {
          username: ban.username,
          caseId: ban.case_id,
        })
      )
      form.reset()
      props.onClose()
      await queryClient.invalidateQueries({ queryKey: CYBERGUARD_QUERY_KEY })
    },
    onError: (error) => handleServerError(error, t('The ban failed.')),
  })
  const user = props.user
  const close = () => {
    if (mutation.isPending) return
    form.reset()
    props.onClose()
  }

  return (
    <Dialog
      open={user !== null}
      onOpenChange={(open) => {
        if (!open) close()
      }}
      title={t('Ban {{username}}', { username: user?.username ?? '' })}
      description={t(
        'The user is moved into the ban group and loses the GPT groups. Unbanning restores the current group. The ban email is sent when ban emails are on.'
      )}
      footer={
        <>
          <Button type='button' variant='outline' onClick={close}>
            {t('Cancel')}
          </Button>
          <Button
            type='submit'
            form={FORM_ID}
            variant='destructive'
            disabled={mutation.isPending}
          >
            {t('Ban user')}
          </Button>
        </>
      }
    >
      {user && (
        <form
          id={FORM_ID}
          className='space-y-4'
          // zod owns range checks so its field messages are shown
          noValidate
          onSubmit={form.handleSubmit((values) =>
            mutation.mutate({ ...values, userId: user.user_id })
          )}
        >
          <dl className='grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 rounded-lg border p-3 text-sm'>
            <dt className='text-muted-foreground'>{t('User')}</dt>
            <dd>
              {user.username} · UID {user.user_id}
            </dd>
            <dt className='text-muted-foreground'>{t('Current group')}</dt>
            <dd>
              <GroupBadge group={user.current_group} />
            </dd>
            <dt className='text-muted-foreground'>
              {t('Flagged sessions (7 / 30 days)')}
            </dt>
            <dd className='tabular-nums'>
              {user.units_7d} / {user.units_30d}
            </dd>
          </dl>
          <BanTermsFields
            form={form}
            idPrefix='cyberguard-ban'
            disabled={mutation.isPending}
            reasonPlaceholder={t('Manual ban')}
          />
        </form>
      )}
    </Dialog>
  )
}

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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { getGroups } from '@/features/users/api'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import { CYBERGUARD_QUERY_KEY, adoptCyberGuardBan } from '../api'
import { getAdoptFormSchema, type AdoptFormValues } from '../lib/forms'
import type { CyberGuardAdoptableUser } from '../types'
import { BanTermsFields } from './ban-terms-fields'

/** Imports one existing manual ban; the group is not changed and no email is sent. */
export function AdoptForm(props: {
  user: CyberGuardAdoptableUser
  banGroup: string
  onDone: () => void
  onCancel: () => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const groupsQuery = useQuery({
    queryKey: ['groups'],
    queryFn: async () => requireServerSuccess(await getGroups()).data ?? [],
  })
  const groupOptions = [
    ...new Set([
      props.user.suggested_original_group,
      ...(groupsQuery.data ?? []),
    ]),
  ].filter((group) => group && group !== props.banGroup)
  const form = useForm<AdoptFormValues>({
    resolver: zodResolver(getAdoptFormSchema(t)),
    defaultValues: {
      original_group: props.user.suggested_original_group,
      hours: '0',
      reason: '',
    },
  })
  const mutation = useMutation({
    mutationFn: (values: AdoptFormValues) =>
      adoptCyberGuardBan({
        user_id: props.user.user_id,
        original_group: values.original_group,
        hours: Number(values.hours),
        reason: values.reason,
      }),
    onSuccess: async (ban) => {
      toast.success(
        t('{{username}} imported as {{caseId}}.', {
          username: ban.username,
          caseId: ban.case_id,
        })
      )
      props.onDone()
      await queryClient.invalidateQueries({ queryKey: CYBERGUARD_QUERY_KEY })
    },
    onError: (error) => handleServerError(error, t('The import failed.')),
  })
  const groupError = form.formState.errors.original_group

  return (
    <form
      className='space-y-4 rounded-lg border p-4'
      noValidate
      onSubmit={form.handleSubmit((values) => mutation.mutate(values))}
    >
      <p className='text-sm font-medium'>
        {t('Import {{username}} (UID {{id}})', {
          username: props.user.username,
          id: props.user.user_id,
        })}
      </p>
      <Field data-invalid={Boolean(groupError)}>
        <FieldLabel htmlFor='cyberguard-adopt-group'>
          {t('Group to restore on unban')}
        </FieldLabel>
        <NativeSelect
          id='cyberguard-adopt-group'
          className='w-full'
          disabled={mutation.isPending}
          aria-invalid={Boolean(groupError)}
          {...form.register('original_group')}
        >
          {groupOptions.map((group) => (
            <NativeSelectOption key={group} value={group}>
              {group}
            </NativeSelectOption>
          ))}
        </NativeSelect>
        <FieldDescription>
          {t(
            'Pre-filled from "原分组:" in the user remark, otherwise default.'
          )}
        </FieldDescription>
        <FieldError errors={[groupError]} />
      </Field>
      <BanTermsFields
        form={form}
        idPrefix='cyberguard-adopt'
        disabled={mutation.isPending}
        reasonPlaceholder={t('Imported existing ban')}
      />
      <div className='flex justify-end gap-2'>
        <Button
          type='button'
          variant='outline'
          disabled={mutation.isPending}
          onClick={props.onCancel}
        >
          {t('Back')}
        </Button>
        <Button type='submit' disabled={mutation.isPending}>
          {t('Import ban')}
        </Button>
      </div>
    </form>
  )
}

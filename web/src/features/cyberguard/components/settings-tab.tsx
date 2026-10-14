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
import { useState, type ReactNode } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { ErrorState } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Form } from '@/components/ui/form'
import { Skeleton } from '@/components/ui/skeleton'
import { handleServerError } from '@/lib/handle-server-error'

import {
  CYBERGUARD_QUERY_KEY,
  fetchCyberGuardSettings,
  updateCyberGuardSettings,
} from '../api'
import {
  getSettingsFormSchema,
  settingsFormToPayload,
  settingsToFormValues,
  type SettingsFormValues,
} from '../lib/forms'
import type { CyberGuardSettings } from '../types'
import { SettingsModeField } from './settings-mode-field'
import {
  SettingsBanFields,
  SettingsCategoriesField,
} from './settings-rule-fields'
import { SettingsSourceFields } from './settings-source-fields'
import { SettingsTextField } from './settings-text-field'
import { SettingsTiersField } from './settings-tiers-field'

export function SettingsTab() {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: [...CYBERGUARD_QUERY_KEY, 'settings'],
    queryFn: fetchCyberGuardSettings,
  })
  if (query.isLoading) return <Skeleton className='h-96 w-full' />
  if (query.isError || !query.data) {
    return (
      <ErrorState
        title={t('Could not load CyberGuard settings')}
        description={query.error?.message}
        onRetry={() => void query.refetch()}
      />
    )
  }
  return <SettingsForm settings={query.data} />
}

function Section(props: {
  title: string
  description: string
  children: ReactNode
}) {
  return (
    <Card data-card-hover='false'>
      <CardHeader>
        <CardTitle>{props.title}</CardTitle>
        <CardDescription>{props.description}</CardDescription>
      </CardHeader>
      <CardContent className='space-y-4'>{props.children}</CardContent>
    </Card>
  )
}

function SettingsForm(props: { settings: CyberGuardSettings }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [pendingEnforce, setPendingEnforce] =
    useState<SettingsFormValues | null>(null)
  const form = useForm<SettingsFormValues>({
    resolver: zodResolver(getSettingsFormSchema(t)),
    defaultValues: settingsToFormValues(props.settings),
  })
  const save = useMutation({
    mutationFn: (values: SettingsFormValues) =>
      updateCyberGuardSettings(settingsFormToPayload(values)),
    onSuccess: async (saved) => {
      toast.success(t('CyberGuard settings saved.'))
      setPendingEnforce(null)
      form.reset(settingsToFormValues(saved))
      await queryClient.invalidateQueries({ queryKey: CYBERGUARD_QUERY_KEY })
    },
    onError: (error) => {
      setPendingEnforce(null)
      handleServerError(error, t('Could not save CyberGuard settings.'))
    },
  })
  const control = form.control

  return (
    <Form {...form}>
      <form
        className='space-y-5'
        // zod owns range checks so its field messages are shown
        noValidate
        onSubmit={form.handleSubmit((values) => {
          if (values.mode === 'enforce' && props.settings.mode !== 'enforce') {
            setPendingEnforce(values)
            return
          }
          save.mutate(values)
        })}
      >
        <Section
          title={t('Mode')}
          description={t(
            'Turning CyberGuard on starts counting from now; earlier refusals are not backfilled.'
          )}
        >
          <SettingsModeField control={control} />
        </Section>
        <Section
          title={t('Detection source')}
          description={t(
            'CyberGuard reads refusals from the sub2api admin API and matches them to new-api usage logs by upstream request ID.'
          )}
        >
          <SettingsSourceFields
            control={control}
            hasAdminKey={props.settings.has_sub2api_admin_api_key}
          />
        </Section>
        <Section
          title={t('Counting rules')}
          description={t(
            'Hits are counted as flagged sessions. Requests in the same conversation count once.'
          )}
        >
          <SettingsCategoriesField control={control} />
          <SettingsTextField
            control={control}
            name='nosession_merge_seconds'
            type='number'
            min={0}
            max={3600}
            label={t('Merge window without a session (seconds)')}
            description={t(
              'Requests without a session fingerprint from the same token and model within this window count as one session, which absorbs client retries. 0 counts every hit.'
            )}
          />
          <SettingsTiersField form={form} />
        </Section>
        <Section
          title={t('Ban handling')}
          description={t(
            'Administrators and exempt users are never banned. Unbanning restores the group the user had before the ban.'
          )}
        >
          <SettingsBanFields control={control} />
        </Section>
        <div className='flex justify-end gap-2'>
          <Button
            type='button'
            variant='outline'
            disabled={save.isPending || !form.formState.isDirty}
            onClick={() => form.reset()}
          >
            {t('Reset')}
          </Button>
          <Button type='submit' disabled={save.isPending}>
            {save.isPending ? t('Saving...') : t('Save settings')}
          </Button>
        </div>
      </form>
      <ConfirmDialog
        open={pendingEnforce !== null}
        onOpenChange={(open) => {
          if (!open && !save.isPending) setPendingEnforce(null)
        }}
        title={t('Turn on automatic bans?')}
        desc={t(
          'Users who reach a tier will be moved into the ban group and emailed without review. Open proposals are not applied by themselves; apply or dismiss them under Ban records.'
        )}
        confirmText={t('Turn on automatic bans')}
        destructive
        isLoading={save.isPending}
        handleConfirm={() => {
          if (pendingEnforce && !save.isPending) save.mutate(pendingEnforce)
        }}
      />
    </Form>
  )
}

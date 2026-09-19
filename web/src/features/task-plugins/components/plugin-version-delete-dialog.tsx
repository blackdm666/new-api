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
import { handleServerError } from '@/lib/handle-server-error'

import { deleteTaskPluginVersion, TaskPluginUsageError } from '../api'
import type { TaskPluginDeleteResult, TaskPluginUsage } from '../types'

type PluginVersionDeleteDialogProps = {
  pluginKey: string
  name: string
  version: string
  active: boolean
  hasFactoryFallback: boolean
  onClose: () => void
  onDeleted?: (result: TaskPluginDeleteResult | null) => void
}

export function PluginVersionDeleteDialog(
  props: PluginVersionDeleteDialogProps
) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [usage, setUsage] = useState<TaskPluginUsage | null>(null)
  const remove = useMutation({
    mutationFn: () =>
      deleteTaskPluginVersion(props.pluginKey, props.version, Boolean(usage)),
    retry: false,
    onSuccess: (result) => {
      if (result?.promoted_version) {
        toast.success(
          t(
            'Deleted version {{version}}. Current version is now {{currentVersion}}.',
            {
              version: props.version,
              currentVersion: result.promoted_version,
            }
          )
        )
      } else if (result?.factory_fallback) {
        toast.success(
          t('Deleted version {{version}}. The built-in plugin is retained.', {
            version: props.version,
          })
        )
      } else if (result?.plugin_removed) {
        toast.success(
          t('Deleted version {{version}}. No custom versions remain.', {
            version: props.version,
          })
        )
      } else {
        toast.success(
          t('Deleted version {{version}}.', { version: props.version })
        )
      }
      props.onDeleted?.(result)
      props.onClose()
      queryClient.removeQueries({
        queryKey: ['task-plugin', props.pluginKey, props.version],
        exact: true,
      })
      if (result?.plugin_removed) {
        queryClient.removeQueries({
          queryKey: ['task-plugin', props.pluginKey],
        })
      } else {
        void queryClient.invalidateQueries({
          queryKey: ['task-plugin', props.pluginKey],
          exact: true,
        })
      }
      void queryClient.invalidateQueries({ queryKey: ['task-plugins'] })
      void queryClient.invalidateQueries({
        queryKey: ['task-plugin-versions', props.pluginKey],
      })
      void queryClient.invalidateQueries({ queryKey: ['task-plugin-options'] })
    },
    onError: (error) => {
      if (error instanceof TaskPluginUsageError) {
        setUsage(error.usage)
        return
      }
      handleServerError(error)
    },
  })

  return (
    <ConfirmDialog
      open
      onOpenChange={(open) => {
        if (!open && !remove.isPending) props.onClose()
      }}
      title={usage ? t('Plugin is still in use') : t('Delete plugin version?')}
      destructive
      isLoading={remove.isPending}
      confirmText={usage ? t('Force operation') : t('Delete')}
      handleConfirm={() => {
        if (!remove.isPending) remove.mutate()
      }}
      desc={
        <div className='space-y-2'>
          <p className='break-words'>
            {t('Delete {{name}} ({{key}}), version {{version}}?', {
              name: props.name,
              key: props.pluginKey,
              version: props.version,
            })}
          </p>
          <p>{t('Only the selected version will be deleted.')}</p>
          {props.active && (
            <p>
              {t(
                'Deleting the current version switches to another installed version when available.'
              )}
            </p>
          )}
          {props.active && !props.hasFactoryFallback && (
            <p>
              {t(
                'If no versions remain, historical tasks using this plugin may become unavailable.'
              )}
            </p>
          )}
          {usage && (
            <>
              <p>
                {t(
                  '{{count}} enabled channels and {{tasks}} in-flight tasks still use this plugin.',
                  {
                    count: usage.channels.length,
                    tasks: usage.in_flight_count,
                  }
                )}
              </p>
              {usage.channels.length > 0 && (
                <ul className='max-h-36 list-inside list-disc overflow-y-auto'>
                  {usage.channels.map((channel) => (
                    <li key={channel.id}>
                      #{channel.id} {channel.name}
                    </li>
                  ))}
                </ul>
              )}
            </>
          )}
        </div>
      }
    />
  )
}

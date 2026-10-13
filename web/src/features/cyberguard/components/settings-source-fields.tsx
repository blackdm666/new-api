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
import type { Control } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'

import type { SettingsFormValues } from '../lib/forms'
import { SettingsTextField } from './settings-text-field'

/** Where refusals come from: the sub2api admin API and the channels it serves. */
export function SettingsSourceFields(props: {
  control: Control<SettingsFormValues>
  hasAdminKey: boolean
}) {
  const { t } = useTranslation()
  return (
    <div className='grid gap-4 md:grid-cols-2'>
      <SettingsTextField
        control={props.control}
        name='sub2api_base_url'
        label={t('sub2api address')}
        description={t('Reached from this server, e.g. http://sub2api:8080')}
      />
      <SettingsTextField
        control={props.control}
        name='sub2api_admin_api_key'
        type='password'
        label={
          <span className='flex items-center gap-2'>
            {t('sub2api admin API key')}
            <Badge variant={props.hasAdminKey ? 'secondary' : 'warning'}>
              {props.hasAdminKey ? t('Saved') : t('Not set')}
            </Badge>
          </span>
        }
        placeholder={
          props.hasAdminKey
            ? t('Leave blank to keep the saved key')
            : t('Enter the admin API key')
        }
        description={t(
          'Write-only: the saved key is never shown again. Polls are skipped until a key is saved.'
        )}
      />
      <SettingsTextField
        control={props.control}
        name='sub2api_api_key_ids'
        label={t('sub2api API key IDs')}
        description={t(
          'IDs of the sub2api keys used by new-api channels, comma separated. Only their refusals are read.'
        )}
      />
      <SettingsTextField
        control={props.control}
        name='protected_channel_ids'
        label={t('Protected channel IDs')}
        description={t(
          'new-api channels that route to the sub2api pool, comma separated. Refusals are attributed through usage logs of these channels.'
        )}
      />
      <SettingsTextField
        control={props.control}
        name='poll_interval_seconds'
        type='number'
        min={15}
        max={3600}
        label={t('Poll interval (seconds)')}
        description={t(
          '15–3600. Runs on the master node, at most every 15 seconds.'
        )}
      />
      <SettingsTextField
        control={props.control}
        name='lookback_minutes'
        type='number'
        min={1}
        max={360}
        label={t('Lookback (minutes)')}
        description={t(
          '1–360. Each poll re-reads this window so late rows are not missed. Nothing older is backfilled when CyberGuard is turned on.'
        )}
      />
    </div>
  )
}

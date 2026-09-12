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
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

import { SettingsControlGroup } from '../components/settings-form-layout'

export type SMTPProfileState = 'enabled' | 'pending' | 'disabled' | 'error'

type Props = {
  title: string
  description: string
  state: SMTPProfileState
  status?: string
}

const STATE_CLASSES: Record<SMTPProfileState, string> = {
  enabled: 'border-emerald-500/40 bg-emerald-500/10 text-emerald-600',
  pending: 'border-amber-500/40 bg-amber-500/10 text-amber-600',
  disabled: 'border-zinc-500/40 bg-zinc-500/10 text-zinc-500',
  error: 'border-red-500/40 bg-red-500/10 text-red-600',
}

export function SMTPProfileStatusCard(props: Props) {
  const { t } = useTranslation()
  let fallbackStatus = t('Disabled')
  if (props.state === 'enabled') {
    fallbackStatus = t('Enabled')
  }
  if (props.state === 'pending') {
    fallbackStatus = t('Pending test')
  }
  if (props.state === 'error') {
    fallbackStatus = t('Error')
  }

  return (
    <SettingsControlGroup className='flex flex-col justify-between gap-3 space-y-0 sm:flex-row sm:items-center'>
      <div className='min-w-0 space-y-1'>
        <p className='text-sm font-medium'>{props.title}</p>
        <p className='text-muted-foreground text-xs'>{props.description}</p>
      </div>
      <Badge
        variant='outline'
        data-state={props.state}
        className={cn('shrink-0', STATE_CLASSES[props.state])}
      >
        {props.status || fallbackStatus}
      </Badge>
    </SettingsControlGroup>
  )
}

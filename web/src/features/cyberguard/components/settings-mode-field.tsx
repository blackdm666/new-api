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

import { FormField, FormItem } from '@/components/ui/form'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { cn } from '@/lib/utils'

import type { SettingsFormValues } from '../lib/forms'
import { modeLabel } from '../lib/labels'
import type { CyberGuardMode } from '../types'

const MODES: CyberGuardMode[] = ['off', 'observe', 'enforce']

export function SettingsModeField(props: {
  control: Control<SettingsFormValues>
}) {
  const { t } = useTranslation()
  const descriptions: Record<CyberGuardMode, string> = {
    off: t('Nothing is polled or counted.'),
    observe: t(
      'Record and count refusals, and create ban proposals for review. No group changes, no email.'
    ),
    enforce: t(
      'Move users who reach a tier into the ban group and send the ban email.'
    ),
  }
  return (
    <FormField
      control={props.control}
      name='mode'
      render={({ field }) => (
        <FormItem>
          <div id='cyberguard-mode-label' className='sr-only'>
            {t('Mode')}
          </div>
          <RadioGroup
            aria-labelledby='cyberguard-mode-label'
            value={field.value}
            onValueChange={(value) => field.onChange(value as CyberGuardMode)}
            className='grid gap-2 md:grid-cols-3'
          >
            {MODES.map((mode) => (
              <Label
                key={mode}
                htmlFor={`cyberguard-mode-${mode}`}
                className={cn(
                  'flex cursor-pointer items-start gap-3 rounded-lg border p-3 font-normal',
                  field.value === mode && 'border-primary bg-primary/5'
                )}
              >
                <RadioGroupItem
                  id={`cyberguard-mode-${mode}`}
                  value={mode}
                  className='mt-0.5'
                />
                <span className='space-y-1'>
                  <span className='block text-sm font-medium'>
                    {modeLabel(mode, t)}
                  </span>
                  <span className='text-muted-foreground block text-xs leading-5'>
                    {descriptions[mode]}
                  </span>
                </span>
              </Label>
            ))}
          </RadioGroup>
        </FormItem>
      )}
    />
  )
}

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
import { Plus, Trash2 } from 'lucide-react'
import { useFieldArray, type UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

import type { SettingsFormValues } from '../lib/forms'

const MAX_TIERS = 5
const TIER_FIELDS = ['window_days', 'min_units', 'ban_hours'] as const

/** Editable ban tiers: "N flagged sessions within D days → ban for H hours". */
export function SettingsTiersField(props: {
  form: UseFormReturn<SettingsFormValues>
}) {
  const { t } = useTranslation()
  const tiers = useFieldArray({ control: props.form.control, name: 'tiers' })
  const errors = props.form.formState.errors.tiers
  const headers: Record<(typeof TIER_FIELDS)[number], string> = {
    window_days: t('Within (days)'),
    min_units: t('Flagged sessions at least'),
    ban_hours: t('Ban hours (0 = until unban)'),
  }
  const limits = {
    window_days: { min: 1, max: 90 },
    min_units: { min: 1, max: 50 },
    ban_hours: { min: 0, max: 8760 },
  }

  return (
    <fieldset className='space-y-2'>
      <legend className='text-sm font-medium'>{t('Ban tiers')}</legend>
      <p className='text-muted-foreground text-sm'>
        {t(
          'When several tiers match, the strictest wins: a ban until manual unban beats any timed ban, otherwise the longer ban wins. An active ban is extended when a stricter tier is reached.'
        )}
      </p>
      <div className='space-y-2 rounded-lg border p-3'>
        <div className='text-muted-foreground hidden grid-cols-[2rem_repeat(3,minmax(0,1fr))_2rem] gap-2 text-xs sm:grid'>
          <span>#</span>
          {TIER_FIELDS.map((field) => (
            <span key={field}>{headers[field]}</span>
          ))}
          <span />
        </div>
        {tiers.fields.map((tier, index) => (
          <div
            key={tier.id}
            className='grid grid-cols-[2rem_repeat(3,minmax(0,1fr))_2rem] items-center gap-2'
          >
            <span className='text-muted-foreground text-sm tabular-nums'>
              {index + 1}
            </span>
            {TIER_FIELDS.map((field) => (
              <Input
                key={field}
                type='number'
                inputMode='numeric'
                min={limits[field].min}
                max={limits[field].max}
                aria-label={`${headers[field]} #${index + 1}`}
                aria-invalid={Boolean(errors?.[index]?.[field])}
                {...props.form.register(`tiers.${index}.${field}`)}
              />
            ))}
            <Button
              type='button'
              variant='ghost'
              size='icon-sm'
              aria-label={t('Remove tier {{n}}', { n: index + 1 })}
              disabled={tiers.fields.length <= 1}
              onClick={() => tiers.remove(index)}
            >
              <Trash2 aria-hidden='true' />
            </Button>
          </div>
        ))}
        {errors && (
          <p className='text-destructive text-sm'>
            {errors.root?.message ??
              errors.message ??
              t('Days 1–90, sessions 1–50, hours 0–8760.')}
          </p>
        )}
        <Button
          type='button'
          variant='outline'
          size='sm'
          disabled={tiers.fields.length >= MAX_TIERS}
          onClick={() =>
            tiers.append({ window_days: '30', min_units: '3', ban_hours: '0' })
          }
        >
          <Plus aria-hidden='true' />
          {t('Add tier')}
        </Button>
      </div>
    </fieldset>
  )
}

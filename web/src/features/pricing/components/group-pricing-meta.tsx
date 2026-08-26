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
import { cn } from '@/lib/utils'

type GroupPricingMetaProps = {
  group: string
  ratio: number
  description?: string
  className?: string
}

function getVisibleGroupDescription(
  group: string,
  description?: string
): string {
  const value = description?.trim() ?? ''
  if (
    !value ||
    value.toLocaleLowerCase() === group.trim().toLocaleLowerCase()
  ) {
    return ''
  }
  return value
}

export function GroupPricingMeta(props: GroupPricingMetaProps) {
  const description = getVisibleGroupDescription(props.group, props.description)

  return (
    <span
      className={cn(
        'ml-auto inline-flex min-w-0 max-w-full flex-wrap items-center justify-end gap-2',
        props.className
      )}
    >
      {description && (
        <span
          className='max-w-full rounded-md border border-amber-500/35 bg-amber-500/10 px-2.5 py-1 text-xs leading-tight font-semibold break-words text-amber-700 shadow-sm dark:border-amber-400/35 dark:bg-amber-400/10 dark:text-amber-300'
          title={description}
        >
          {description}
        </span>
      )}
      <span className='rounded-md border border-emerald-500/35 bg-emerald-500/10 px-2.5 py-1 font-mono text-xs leading-none font-extrabold tracking-tight text-emerald-700 tabular-nums shadow-sm dark:border-emerald-400/35 dark:bg-emerald-400/10 dark:text-emerald-300'>
        {props.ratio}x
      </span>
    </span>
  )
}

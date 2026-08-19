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
        'inline-flex max-w-full flex-wrap items-center justify-end gap-x-1.5 gap-y-0.5 rounded-full border border-emerald-500/35 bg-emerald-500/10 px-2.5 py-1 text-emerald-700 shadow-sm dark:border-emerald-400/35 dark:bg-emerald-400/10 dark:text-emerald-300',
        props.className
      )}
    >
      <span className='font-mono text-xs leading-none font-extrabold tracking-tight tabular-nums'>
        {props.ratio}x
      </span>
      {description && (
        <>
          <span
            aria-hidden='true'
            className='text-emerald-500/70 dark:text-emerald-300/60'
          >
            ·
          </span>
          <span
            className='max-w-full text-xs leading-tight font-semibold break-words'
            title={description}
          >
            {description}
          </span>
        </>
      )}
    </span>
  )
}

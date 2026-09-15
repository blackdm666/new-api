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
import { type Ref, useCallback, useEffect, useRef, useState } from 'react'

import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { cn } from '@/lib/utils'

type OverflowNoteProps = {
  text: string
  className?: string
}

export function OverflowNote({ text, className }: OverflowNoteProps) {
  const textRef = useRef<HTMLElement | null>(null)
  const [isOverflowing, setIsOverflowing] = useState(false)

  const measureOverflow = useCallback(() => {
    const element = textRef.current
    if (!element) return
    const next = element.scrollWidth > element.clientWidth + 1
    setIsOverflowing((current) => (current === next ? current : next))
  }, [])

  useEffect(() => {
    measureOverflow()
    const element = textRef.current
    if (!element || typeof ResizeObserver === 'undefined') return

    const observer = new ResizeObserver(measureOverflow)
    observer.observe(element)
    return () => observer.disconnect()
  }, [isOverflowing, measureOverflow, text])

  const textClassName = cn(
    'block min-w-0 max-w-full truncate text-left',
    className
  )

  if (!isOverflowing) {
    return (
      <span ref={textRef} className={textClassName}>
        {text}
      </span>
    )
  }

  return (
    <Popover>
      <PopoverTrigger
        render={
          <button
            ref={textRef as Ref<HTMLButtonElement>}
            type='button'
            className={cn(
              textClassName,
              'hover:text-foreground focus-visible:ring-ring cursor-pointer rounded-sm focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none'
            )}
          />
        }
      >
        {text}
      </PopoverTrigger>
      <PopoverContent
        side='top'
        align='start'
        className='w-fit max-w-sm break-words whitespace-pre-wrap'
      >
        {text}
      </PopoverContent>
    </Popover>
  )
}

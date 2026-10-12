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
import { ArrowRight } from 'lucide-react'
import { useRef, useState } from 'react'

import {
  NavigationMenu,
  NavigationMenuContent,
  NavigationMenuItem,
  NavigationMenuLink,
  NavigationMenuList,
  NavigationMenuTrigger,
} from '@/components/ui/navigation-menu'
import { trackNavClick, type NavClickLocation } from '@/lib/analytics'
import { cn } from '@/lib/utils'

import type { TopNavLink } from '../types'

const MENU_VALUE = 'menu'

type TopNavDropdownProps = {
  link: TopNavLink
  /** Classes that match the sibling top navigation links */
  triggerClassName?: string
  /** Reported as `nav_location` when the trigger itself is clicked */
  navLocation: NavClickLocation
}

/**
 * Desktop top navigation entry with secondary destinations.
 *
 * The trigger stays a real link to `link.href` (full page load): hovering or
 * pressing ArrowDown opens the dropdown, clicking or pressing Enter follows the
 * link. On touch screens the first tap opens the dropdown and the second tap
 * follows the link. Every destination is a plain `<a href>` in the current tab.
 */
export function TopNavDropdown(props: TopNavDropdownProps) {
  const [value, setValue] = useState<string | null>(null)
  const pointerTypeRef = useRef('')
  const items = props.link.items ?? []

  return (
    <NavigationMenu
      render={<div />}
      align='center'
      delay={80}
      closeDelay={180}
      value={value}
      onValueChange={setValue}
      className='flex-none'
    >
      <NavigationMenuList>
        <NavigationMenuItem value={MENU_VALUE}>
          <NavigationMenuTrigger
            nativeButton={false}
            render={<a href={props.link.href} />}
            // Announce a link (it navigates), not the button role Base UI adds by default.
            role={undefined}
            onPointerDown={(event) => {
              pointerTypeRef.current = event.pointerType
            }}
            onClick={(event) => {
              const isTouchTap =
                event.detail > 0 && pointerTypeRef.current === 'touch'
              if (isTouchTap && value !== MENU_VALUE) {
                event.preventDefault()
                return
              }
              if (props.link.trackId) {
                trackNavClick(props.link.trackId, props.navLocation)
              }
            }}
            className={cn(
              'h-auto bg-transparent hover:bg-transparent focus:bg-transparent data-open:bg-transparent data-open:hover:bg-transparent data-open:focus:bg-transparent data-popup-open:bg-transparent data-popup-open:hover:bg-transparent',
              props.triggerClassName
            )}
          >
            {props.link.title}
          </NavigationMenuTrigger>
          <NavigationMenuContent className='w-[min(25rem,calc(100vw-2rem))] p-2'>
            <ul className='grid gap-1'>
              {items.map((item) => (
                <li key={item.href}>
                  <NavigationMenuLink
                    href={item.href}
                    onClick={() => {
                      if (item.trackId) trackNavClick(item.trackId, 'dropdown')
                    }}
                    className='group/item grid grid-cols-[2.25rem_minmax(0,1fr)_1rem] items-center gap-3 p-3'
                  >
                    <span className='bg-primary/10 text-primary flex size-9 items-center justify-center rounded-lg'>
                      {item.icon ? (
                        <item.icon className='size-5' aria-hidden='true' />
                      ) : null}
                    </span>
                    <span className='flex min-w-0 flex-col gap-0.5'>
                      <span className='text-foreground text-sm font-semibold'>
                        {item.title}
                      </span>
                      {item.description ? (
                        <span className='text-muted-foreground text-xs leading-snug'>
                          {item.description}
                        </span>
                      ) : null}
                    </span>
                    <ArrowRight
                      aria-hidden='true'
                      className='text-muted-foreground size-4 transition-transform group-hover/item:translate-x-0.5'
                    />
                  </NavigationMenuLink>
                </li>
              ))}
            </ul>
            {props.link.overviewTitle ? (
              <div className='border-border mt-1 flex justify-end border-t pt-1'>
                <NavigationMenuLink
                  href={props.link.href}
                  onClick={() => {
                    if (props.link.trackId) {
                      trackNavClick(props.link.trackId, 'dropdown')
                    }
                  }}
                  className='text-muted-foreground hover:text-foreground px-3 text-xs'
                >
                  {props.link.overviewTitle}
                  <ArrowRight aria-hidden='true' className='size-3.5' />
                </NavigationMenuLink>
              </div>
            ) : null}
          </NavigationMenuContent>
        </NavigationMenuItem>
      </NavigationMenuList>
    </NavigationMenu>
  )
}

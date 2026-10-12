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
import { Link } from '@tanstack/react-router'
import { Menu } from 'lucide-react'
import { useMemo } from 'react'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { trackNavClick } from '@/lib/analytics'
import { cn } from '@/lib/utils'

import { getExternalLinkProps } from '../lib/top-nav-link'
import type { TopNavLink } from '../types'
import { TopNavDropdown } from './top-nav-dropdown'

type TopNavProps = React.HTMLAttributes<HTMLElement> & {
  links: TopNavLink[]
}

/**
 * 顶部导航栏组件
 * 在大屏幕显示水平导航，在小屏幕显示下拉菜单
 */
export function TopNav({ className, links, ...props }: TopNavProps) {
  // 规范化链接，确保所有可选属性都有默认值
  const normalizedLinks = useMemo(
    () =>
      links.map((link) => ({
        isActive: false,
        disabled: false,
        external: false,
        ...link,
      })),
    [links]
  )

  return (
    <>
      {/* 移动端下拉菜单 */}
      <div className='lg:hidden'>
        <DropdownMenu modal={false}>
          <DropdownMenuTrigger
            render={<Button size='icon' variant='outline' className='size-7' />}
          >
            <Menu />
          </DropdownMenuTrigger>
          <DropdownMenuContent side='bottom' align='start'>
            {normalizedLinks.map((link) =>
              link.items?.length ? (
                <DropdownMenuGroup key={`${link.title}-${link.href}`}>
                  <DropdownMenuLabel>{link.title}</DropdownMenuLabel>
                  {link.items.map((item) => (
                    <DropdownMenuItem
                      key={item.href}
                      render={
                        <a
                          href={item.href}
                          className='text-muted-foreground'
                          onClick={() => {
                            if (item.trackId) {
                              trackNavClick(item.trackId, 'mobile_menu')
                            }
                          }}
                        >
                          {item.title}
                        </a>
                      }
                    />
                  ))}
                </DropdownMenuGroup>
              ) : (
                <DropdownMenuItem
                  key={`${link.title}-${link.href}`}
                  render={
                    link.external ? (
                      <a
                        href={link.href}
                        {...getExternalLinkProps(link)}
                        className={
                          !link.isActive ? 'text-muted-foreground' : ''
                        }
                      >
                        {link.title}
                      </a>
                    ) : (
                      <Link
                        to={link.href}
                        className={
                          !link.isActive ? 'text-muted-foreground' : ''
                        }
                        disabled={link.disabled}
                      >
                        {link.title}
                      </Link>
                    )
                  }
                />
              )
            )}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      {/* 桌面端水平导航 */}
      <nav
        className={cn(
          'hidden items-center space-x-4 lg:flex lg:space-x-4 xl:space-x-6',
          className
        )}
        {...props}
      >
        {normalizedLinks.map((link) => {
          const linkClassName = `hover:text-primary text-sm font-medium transition-colors ${link.isActive ? '' : 'text-muted-foreground'}`
          if (link.items?.length) {
            return (
              <TopNavDropdown
                key={`${link.title}-${link.href}`}
                link={link}
                navLocation='console_header'
                triggerClassName={cn(
                  linkClassName,
                  'data-open:text-primary rounded-md px-0 py-0'
                )}
              />
            )
          }
          return link.external ? (
            <a
              key={`${link.title}-${link.href}`}
              href={link.href}
              {...getExternalLinkProps(link)}
              className={linkClassName}
            >
              {link.title}
            </a>
          ) : (
            <Link
              key={`${link.title}-${link.href}`}
              to={link.href}
              disabled={link.disabled}
              className={linkClassName}
            >
              {link.title}
            </Link>
          )
        })}
      </nav>
    </>
  )
}

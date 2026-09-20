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
import { useQuery } from '@tanstack/react-query'
import { Check, ChevronsUpDown } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Command,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { useDebounce } from '@/hooks/use-debounce'
import { cn } from '@/lib/utils'

import { getUserInviterOptions } from '../api'
import type { UserInviterOption } from '../types'

type UserInviterSelectorProps = {
  targetUserId: number
  value: number
  onValueChange: (value: number) => void
}

function inviterOptionLabel(option: UserInviterOption): string {
  return option.display_name || option.username
}

export function UserInviterSelector(props: UserInviterSelectorProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [searchValue, setSearchValue] = useState('')
  const debouncedSearchValue = useDebounce(searchValue, 300)
  const selectedUserId = props.value > 0 ? props.value : undefined
  const { data, isFetching } = useQuery({
    queryKey: [
      'user-inviter-options',
      props.targetUserId,
      selectedUserId,
      debouncedSearchValue,
    ],
    queryFn: () =>
      getUserInviterOptions({
        targetUserId: props.targetUserId,
        selectedUserId,
        keyword: debouncedSearchValue,
      }),
    enabled: props.targetUserId > 0,
    staleTime: 30 * 1000,
  })
  const options = data?.data ?? []
  const selectedOption = options.find((option) => option.id === props.value)
  let selectedLabel = t('No Inviter')
  if (selectedOption) {
    selectedLabel = inviterOptionLabel(selectedOption)
  } else if (props.value > 0) {
    selectedLabel = `UID ${props.value}`
  }

  const selectValue = (value: number) => {
    props.onValueChange(value)
    setOpen(false)
    setSearchValue('')
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <Button
            type='button'
            variant='outline'
            role='combobox'
            aria-expanded={open}
            className='h-auto min-h-11 w-full justify-between px-3 py-2 text-left font-normal'
          />
        }
      >
        <span className='min-w-0 flex-1'>
          <span className='block truncate'>{selectedLabel}</span>
          {selectedOption && (
            <span className='text-muted-foreground block truncate text-xs'>
              @{selectedOption.username} · UID {selectedOption.id}
              {selectedOption.status !== 1 ? ` · ${t('Disabled')}` : ''}
            </span>
          )}
        </span>
        <ChevronsUpDown
          aria-hidden='true'
          className='ml-2 size-4 shrink-0 opacity-50'
        />
      </PopoverTrigger>
      <PopoverContent
        className='w-[var(--anchor-width)] overflow-hidden p-0'
        onWheel={(event) => event.stopPropagation()}
        onTouchMove={(event) => event.stopPropagation()}
      >
        <Command shouldFilter={false}>
          <CommandInput
            placeholder={t(
              'Search by user ID, username, display name, or email'
            )}
            value={searchValue}
            onValueChange={setSearchValue}
          />
          <CommandList className='max-h-72'>
            <CommandGroup>
              <CommandItem value='no-inviter' onSelect={() => selectValue(0)}>
                <Check
                  aria-hidden='true'
                  className={cn(
                    'size-4',
                    props.value === 0 ? 'opacity-100' : 'opacity-0'
                  )}
                />
                <span>{t('No Inviter')}</span>
              </CommandItem>
              {options.map((option) => (
                <CommandItem
                  key={option.id}
                  value={String(option.id)}
                  onSelect={() => selectValue(option.id)}
                  className='items-start'
                >
                  <Check
                    aria-hidden='true'
                    className={cn(
                      'mt-0.5 size-4',
                      props.value === option.id ? 'opacity-100' : 'opacity-0'
                    )}
                  />
                  <span className='min-w-0 flex-1'>
                    <span className='block truncate font-medium'>
                      {inviterOptionLabel(option)}
                    </span>
                    <span className='text-muted-foreground block truncate text-xs'>
                      @{option.username} · UID {option.id}
                      {option.status !== 1 ? ` · ${t('Disabled')}` : ''}
                    </span>
                  </span>
                </CommandItem>
              ))}
            </CommandGroup>
            {!isFetching && options.length === 0 && searchValue.trim() && (
              <div className='text-muted-foreground px-3 py-6 text-center text-sm'>
                {t('No matching results')}
              </div>
            )}
            {isFetching && (
              <div className='text-muted-foreground px-3 py-3 text-center text-xs'>
                {t('Loading...')}
              </div>
            )}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}

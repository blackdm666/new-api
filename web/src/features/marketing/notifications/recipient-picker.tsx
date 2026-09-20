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
import { Search, X } from 'lucide-react'
import { useDeferredValue, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Field, FieldLabel } from '@/components/ui/field'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from '@/components/ui/input-group'

import { searchNoticeUsers } from './api'
import type { NoticeUser } from './types'

export function NoticeRecipientPicker(props: {
  preview: boolean
  selected: NoticeUser[]
  onChange: (users: NoticeUser[]) => void
  disabled: boolean
}) {
  const { t } = useTranslation()
  const [search, setSearch] = useState('')
  const query = useDeferredValue(search)
  const users = useQuery({
    queryKey: ['user-notices', props.preview, 'users', query],
    queryFn: () => searchNoticeUsers(query, props.preview),
  })
  const toggle = (user: NoticeUser) => {
    if (props.selected.some((item) => item.id === user.id)) {
      props.onChange(props.selected.filter((item) => item.id !== user.id))
    } else {
      props.onChange([...props.selected, user])
    }
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('1. Select recipients')}</CardTitle>
        <CardDescription>
          {t(
            'Only explicitly selected users receive this notice. Disabled accounts can still be notified.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='space-y-4'>
        <Field>
          <FieldLabel htmlFor='notice-user-search' className='sr-only'>
            {t('Find recipients')}
          </FieldLabel>
          <InputGroup>
            <InputGroupInput
              id='notice-user-search'
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder={t('Search UID, username or email')}
              disabled={props.disabled}
            />
            <InputGroupAddon>
              <Search className='size-4' aria-hidden='true' />
            </InputGroupAddon>
          </InputGroup>
        </Field>
        <div
          className='max-h-64 overflow-y-auto rounded-lg border'
          aria-label={t('Recipient search results')}
          aria-busy={users.isFetching}
        >
          {users.isPending && (
            <p className='text-muted-foreground p-4 text-sm'>
              {t('Loading...')}
            </p>
          )}
          {users.isError && (
            <div className='p-4 text-sm' role='alert'>
              {t('Could not load recipients')}{' '}
              <Button
                variant='outline'
                size='sm'
                onClick={() => void users.refetch()}
              >
                {t('Retry')}
              </Button>
            </div>
          )}
          {users.data?.length === 0 && (
            <EmptyState
              className='min-h-24'
              title={t('No matching users')}
              description={t('Try a different UID, username or email.')}
            />
          )}
          {users.data?.map((user) => (
            <label
              key={user.id}
              className='hover:bg-muted/40 flex cursor-pointer items-center gap-3 border-b p-3 last:border-0'
            >
              <Checkbox
                aria-label={t('Select {{name}}', { name: user.username })}
                checked={props.selected.some((item) => item.id === user.id)}
                disabled={props.disabled}
                onCheckedChange={() => toggle(user)}
              />
              <span className='min-w-0 flex-1'>
                <span className='flex flex-wrap items-center gap-2 text-sm font-medium'>
                  {user.display_name}
                  <span className='text-muted-foreground text-xs font-normal'>
                    @{user.username} · UID {user.id}
                  </span>
                </span>
                <span className='text-muted-foreground block truncate text-xs'>
                  {user.email_masked || t('No email address')} · {user.group}
                </span>
              </span>
              {user.disabled && (
                <Badge variant='outline'>{t('Disabled account')}</Badge>
              )}
              {user.skip_reason && (
                <Badge variant='secondary'>{t('Will be skipped')}</Badge>
              )}
            </label>
          ))}
        </div>
        <div className='flex flex-wrap items-center gap-2' aria-live='polite'>
          <span className='text-muted-foreground text-xs'>
            {t('{{count}} selected recipients', {
              count: props.selected.length,
            })}
          </span>
          {props.selected.map((user) => (
            <Badge key={user.id} variant='secondary' className='gap-1 py-1'>
              {user.display_name}
              <Button
                size='icon'
                variant='ghost'
                className='size-5'
                disabled={props.disabled}
                onClick={() => toggle(user)}
                aria-label={t('Remove {{name}}', { name: user.username })}
              >
                <X className='size-3' aria-hidden='true' />
              </Button>
            </Badge>
          ))}
        </div>
        {props.selected.some((user) => user.skip_reason) && (
          <div
            className='text-xs text-amber-700 dark:text-amber-400'
            role='status'
          >
            {t(
              'Users without an email address or with delivery restrictions will be skipped, not silently replaced.'
            )}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

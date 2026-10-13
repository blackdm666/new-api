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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Radar, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { handleServerError } from '@/lib/handle-server-error'

import {
  CYBERGUARD_QUERY_KEY,
  cyberGuardOverviewQuery,
  pollCyberGuardNow,
} from '../api'
import { MODE_VARIANT, modeLabel } from '../lib/labels'
import type { CyberGuardSearch, CyberGuardTab } from '../lib/search'
import { AccountsTab } from './accounts-tab'
import { BansTab } from './bans-tab'
import { EventsTab } from './events-tab'
import { OverviewTab } from './overview-tab'
import { SettingsTab } from './settings-tab'
import { UsersTab } from './users-tab'

/** 风控中心: the root-only CyberGuard admin page. */
export function CyberGuardWorkspace(props: {
  search: CyberGuardSearch
  onSearchChange: (next: CyberGuardSearch) => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const tab = props.search.tab ?? 'overview'
  const overview = useQuery(cyberGuardOverviewQuery)
  const poll = useMutation({
    mutationFn: pollCyberGuardNow,
    onSuccess: async (result) => {
      if (result.status === 'ok') {
        toast.success(
          t(
            'Poll finished: {{fetched}} rows read, {{events}} new events, {{bans}} bans applied.',
            {
              fetched: result.fetched,
              events: result.new_events,
              bans: result.bans_applied,
            }
          )
        )
      } else if (result.status === 'skipped') {
        toast.warning(
          t(
            'Poll skipped: CyberGuard is off or the sub2api admin key is not set.'
          )
        )
      } else {
        toast.error(t('Poll failed: {{error}}', { error: result.error ?? '' }))
      }
      await queryClient.invalidateQueries({ queryKey: CYBERGUARD_QUERY_KEY })
    },
    onError: (error) => handleServerError(error, t('Poll failed.')),
  })
  const mode = overview.data?.mode
  const isMaster = overview.data?.is_master_node ?? false
  const clearUser = () => props.onSearchChange({ tab })

  return (
    <div className='h-full [scrollbar-gutter:stable] overflow-y-auto px-4 py-6 sm:px-8'>
      <div className='mx-auto w-full max-w-[1500px] space-y-5'>
        <header className='flex flex-wrap items-start justify-between gap-3'>
          <div>
            <h1 className='flex flex-wrap items-center gap-2 text-xl font-semibold tracking-tight'>
              {t('CyberGuard')}
              {mode && (
                <StatusBadge
                  size='lg'
                  copyable={false}
                  label={modeLabel(mode, t)}
                  variant={MODE_VARIANT[mode]}
                />
              )}
            </h1>
            <p className='text-muted-foreground mt-1 text-sm'>
              {t(
                'Attributes upstream (OpenAI) content-policy refusals to users and removes GPT groups from repeat offenders.'
              )}
            </p>
          </div>
          <div className='flex gap-2'>
            <Button
              type='button'
              variant='outline'
              onClick={() =>
                void queryClient.invalidateQueries({
                  queryKey: CYBERGUARD_QUERY_KEY,
                })
              }
            >
              <RefreshCw className='size-4' aria-hidden='true' />
              {t('Refresh')}
            </Button>
            <Button
              type='button'
              disabled={!isMaster || poll.isPending}
              title={
                isMaster ? undefined : t('Polls run on the master node only.')
              }
              onClick={() => poll.mutate()}
            >
              <Radar className='size-4' aria-hidden='true' />
              {poll.isPending ? t('Polling...') : t('Poll now')}
            </Button>
          </div>
        </header>

        <Tabs
          value={tab}
          onValueChange={(value) =>
            props.onSearchChange({ tab: value as CyberGuardTab })
          }
          className='gap-4'
        >
          <TabsList className='max-w-full flex-wrap justify-start group-data-horizontal/tabs:h-auto'>
            <TabsTrigger value='overview'>{t('Overview')}</TabsTrigger>
            <TabsTrigger value='users'>{t('Users')}</TabsTrigger>
            <TabsTrigger value='events'>{t('Hit details')}</TabsTrigger>
            <TabsTrigger value='bans'>{t('Ban records')}</TabsTrigger>
            <TabsTrigger value='accounts'>{t('By account')}</TabsTrigger>
            <TabsTrigger value='settings'>{t('Settings')}</TabsTrigger>
          </TabsList>
          <TabsContent value='overview'>
            <OverviewTab />
          </TabsContent>
          <TabsContent value='users'>
            <UsersTab
              onOpenEvents={(userId) =>
                props.onSearchChange({ tab: 'events', userId })
              }
            />
          </TabsContent>
          <TabsContent value='events'>
            <EventsTab userId={props.search.userId} onClearUser={clearUser} />
          </TabsContent>
          <TabsContent value='bans'>
            <BansTab userId={props.search.userId} onClearUser={clearUser} />
          </TabsContent>
          <TabsContent value='accounts'>
            <AccountsTab />
          </TabsContent>
          <TabsContent value='settings'>
            <SettingsTab />
          </TabsContent>
        </Tabs>
      </div>
    </div>
  )
}

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
import { ShieldAlert } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { OverviewStat } from '@/components/overview-stat'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Skeleton } from '@/components/ui/skeleton'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import { cyberGuardOverviewQuery } from '../api'
import { modeLabel } from '../lib/labels'
import { LastPollCard, ReconciliationCard } from './overview-cards'

export function OverviewTab() {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const query = useQuery(cyberGuardOverviewQuery)

  if (query.isLoading) return <Skeleton className='h-48 w-full' />
  if (query.isError) {
    return (
      <ErrorState
        title={t('Could not load the CyberGuard overview')}
        description={query.error.message}
        onRetry={() => void query.refetch()}
      />
    )
  }
  const overview = query.data
  if (!overview) return null
  const counts = overview.counts
  const modeDescriptions = {
    off: t(
      'Nothing is polled, counted or banned. Timed bans that are already active still expire on schedule.'
    ),
    observe: t(
      'Refusals are recorded and counted. A user who reaches a tier gets a ban proposal for you to review; groups are not changed and no email is sent.'
    ),
    enforce: t(
      'A user who reaches a tier is moved into the ban group and receives the ban email. Unbanning restores the original group without an email.'
    ),
  }
  const stats = [
    { label: t('Hits today'), value: counts.hits_today },
    { label: t('Hits (7 days)'), value: counts.hits_7d },
    { label: t('Flagged sessions (7 days)'), value: counts.units_7d },
    { label: t('Flagged users (7 days)'), value: counts.users_flagged_7d },
    { label: t('Banned now'), value: counts.active_bans },
    { label: t('Open proposals'), value: counts.observed_open },
    { label: t('Unattributed (7 days)'), value: counts.unattributed_7d },
    { label: t('Awaiting attribution'), value: counts.pending },
  ]

  return (
    <div className='space-y-5'>
      <Alert>
        <ShieldAlert aria-hidden='true' />
        <AlertTitle>
          {t('Mode: {{mode}}', { mode: modeLabel(overview.mode, t) })}
        </AlertTitle>
        <AlertDescription>
          <p>{modeDescriptions[overview.mode] ?? overview.mode}</p>
          {!overview.is_master_node && (
            <p>
              {t(
                'This server is not the master node. Polls run on the master node only.'
              )}
            </p>
          )}
        </AlertDescription>
      </Alert>
      <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-4'>
        {stats.map((stat) => (
          <OverviewStat
            key={stat.label}
            label={stat.label}
            value={formatNumber(stat.value, locale)}
          />
        ))}
      </div>
      <div className='grid gap-4 lg:grid-cols-2'>
        <LastPollCard poll={overview.last_poll} />
        <ReconciliationCard reconciliation={overview.reconciliation_24h} />
      </div>
    </div>
  )
}

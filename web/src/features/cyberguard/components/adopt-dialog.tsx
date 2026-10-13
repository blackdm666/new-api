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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable, TruncatedCell } from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { GroupBadge } from '@/components/group-badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'

import { CYBERGUARD_QUERY_KEY, fetchCyberGuardAdoptable } from '../api'
import type { CyberGuardAdoptableUser } from '../types'
import { AdoptForm } from './adopt-form'

/**
 * Lists users already sitting in the ban group without a CyberGuard record
 * (manual bans from before CyberGuard) so they can be tracked and unbanned.
 */
export function AdoptDialog(props: {
  open: boolean
  banGroup: string
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const [selected, setSelected] = useState<CyberGuardAdoptableUser | null>(null)
  const query = useQuery({
    queryKey: [...CYBERGUARD_QUERY_KEY, 'adoptable'],
    queryFn: fetchCyberGuardAdoptable,
    enabled: props.open,
  })
  const rows = query.data ?? []

  return (
    <Dialog
      open={props.open}
      onOpenChange={(open) => {
        if (!open) setSelected(null)
        props.onOpenChange(open)
      }}
      title={t('Import existing bans')}
      description={t(
        'Users in the ban group ({{group}}) without a CyberGuard record. Importing keeps their current group, sends no email, and lets an unban restore the group you choose.',
        { group: props.banGroup }
      )}
      contentClassName='sm:max-w-3xl'
    >
      {selected ? (
        <AdoptForm
          key={selected.user_id}
          user={selected}
          banGroup={props.banGroup}
          onCancel={() => setSelected(null)}
          onDone={() => setSelected(null)}
        />
      ) : (
        <AdoptableList
          rows={rows}
          loading={query.isLoading}
          error={query.isError ? query.error.message : null}
          onRetry={() => void query.refetch()}
          onSelect={setSelected}
        />
      )}
    </Dialog>
  )
}

function AdoptableList(props: {
  rows: CyberGuardAdoptableUser[]
  loading: boolean
  error: string | null
  onRetry: () => void
  onSelect: (user: CyberGuardAdoptableUser) => void
}) {
  const { t } = useTranslation()
  if (props.loading) return <Skeleton className='h-32 w-full' />
  if (props.error !== null) {
    return (
      <ErrorState
        className='min-h-32'
        title={t('Could not load users to import')}
        description={props.error}
        onRetry={props.onRetry}
      />
    )
  }
  return (
    <StaticDataTable
      data={props.rows}
      getRowKey={(row) => row.user_id}
      emptyContent={
        <EmptyState
          className='min-h-32'
          title={t('Nothing to import')}
          description={t(
            'Every user in the ban group already has a CyberGuard record.'
          )}
        />
      }
      columns={[
        {
          id: 'user',
          header: t('User'),
          cell: (row) => `${row.username} · UID ${row.user_id}`,
        },
        {
          id: 'remark',
          header: t('Remark'),
          cell: (row) => (
            <TruncatedCell className='max-w-56'>
              {row.remark || '—'}
            </TruncatedCell>
          ),
        },
        {
          id: 'suggested',
          header: t('Suggested original group'),
          cell: (row) => <GroupBadge group={row.suggested_original_group} />,
        },
        {
          id: 'actions',
          header: t('Actions'),
          cell: (row) => (
            <Button
              type='button'
              size='sm'
              variant='outline'
              onClick={() => props.onSelect(row)}
            >
              {t('Import')}
            </Button>
          ),
        },
      ]}
    />
  )
}

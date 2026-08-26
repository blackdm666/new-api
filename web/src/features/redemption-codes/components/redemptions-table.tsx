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
import { getRouteApi } from '@tanstack/react-router'
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  DISABLED_ROW_DESKTOP,
  DISABLED_ROW_MOBILE,
  DataTablePage,
  useDataTable,
} from '@/components/data-table'
import { Input } from '@/components/ui/input'
import { UserInfoDialog } from '@/components/user-info-dialog'
import { useDebounce, useMediaQuery } from '@/hooks'
import { useTableUrlState } from '@/hooks/use-table-url-state'

import { getRedemptions, searchRedemptions } from '../api'
import {
  ERROR_MESSAGES,
  REDEMPTION_STATUS,
  getRedemptionStatusOptions,
} from '../constants'
import { isRedemptionExpired } from '../lib'
import type { Redemption } from '../types'
import { DataTableBulkActions } from './data-table-bulk-actions'
import { useRedemptionsColumns } from './redemptions-columns'
import { RedemptionsMobileList } from './redemptions-mobile-list'
import { useRedemptions } from './redemptions-provider'

const route = getRouteApi('/_authenticated/redemption-codes/')

function isDisabledRedemptionRow(redemption: Redemption) {
  return (
    redemption.status !== REDEMPTION_STATUS.ENABLED ||
    isRedemptionExpired(redemption.expired_time, redemption.status)
  )
}

export function RedemptionsTable() {
  const { t } = useTranslation()
  const { refreshTrigger } = useRedemptions()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const [selectedUserId, setSelectedUserId] = useState<number | null>(null)
  const [userInfoDialogOpen, setUserInfoDialogOpen] = useState(false)

  const handleUserClick = useCallback((userId: number) => {
    setSelectedUserId(userId)
    setUserInfoDialogOpen(true)
  }, [])
  const columns = useRedemptionsColumns(handleUserClick)

  const {
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: route.useSearch(),
    navigate: route.useNavigate(),
    pagination: { defaultPage: 1, defaultPageSize: isMobile ? 10 : 20 },
    globalFilter: { enabled: true, key: 'filter' },
    columnFilters: [
      { columnId: 'name', searchKey: 'name', type: 'string' },
      { columnId: 'code', searchKey: 'code', type: 'string' },
      { columnId: 'id', searchKey: 'id', type: 'string' },
      { columnId: 'status', searchKey: 'status', type: 'array' },
    ],
  })
  const getTextFilter = (columnId: string) =>
    String(columnFilters.find((filter) => filter.id === columnId)?.value ?? '')
  const nameFilter = getTextFilter('name')
  const codeFilter = getTextFilter('code')
  const idFilter = getTextFilter('id')
  const debouncedCodeFilter = useDebounce(codeFilter, 500)
  const debouncedIdFilter = useDebounce(idFilter, 500)
  const statusFilter =
    (columnFilters.find((filter) => filter.id === 'status')?.value as
      | string[]
      | undefined) ?? []
  const statusFilterValue = statusFilter[0] ?? ''

  // Fetch data with React Query
  const { data, isLoading, isFetching } = useQuery({
    queryKey: [
      'redemptions',
      pagination.pageIndex + 1,
      pagination.pageSize,
      globalFilter,
      nameFilter,
      debouncedCodeFilter,
      debouncedIdFilter,
      statusFilterValue,
      refreshTrigger,
    ],
    queryFn: async () => {
      const hasFilter =
        globalFilter?.trim() ||
        nameFilter.trim() ||
        debouncedCodeFilter.trim() ||
        debouncedIdFilter.trim()
      const hasStatusFilter = statusFilterValue !== ''
      const params = {
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
      }

      const result =
        hasFilter || hasStatusFilter
          ? await searchRedemptions({
              ...params,
              name: nameFilter,
              code: debouncedCodeFilter,
              id: debouncedIdFilter,
              // Supports old bookmarked URLs that used the mixed filter.
              keyword: globalFilter,
              status: statusFilterValue,
            })
          : await getRedemptions(params)

      if (!result.success) {
        toast.error(
          result.message ||
            t(
              hasFilter || hasStatusFilter
                ? ERROR_MESSAGES.SEARCH_FAILED
                : ERROR_MESSAGES.LOAD_FAILED
            )
        )
        return { items: [], total: 0 }
      }

      return {
        items: result.data?.items || [],
        total: result.data?.total || 0,
      }
    },
    placeholderData: (previousData) => previousData,
  })

  const redemptions = data?.items || []

  const { table } = useDataTable({
    data: redemptions,
    columns,
    enableRowSelection: true,
    columnFilters,
    globalFilter,
    pagination,
    globalFilterFn: () => true,
    onPaginationChange,
    onGlobalFilterChange,
    onColumnFiltersChange,
    manualPagination: true,
    manualFiltering: true,
    totalCount: data?.total || 0,
    ensurePageInRange,
  })

  const redemptionStatusOptions = useMemo(
    () => getRedemptionStatusOptions(t),
    [t]
  )

  return (
    <>
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={isLoading}
        isFetching={isFetching}
        emptyTitle={t('No Redemption Codes Found')}
        emptyDescription={t(
          'No redemption codes available. Create your first redemption code to get started.'
        )}
        skeletonKeyPrefix='redemptions-skeleton'
        applyHeaderSize
        toolbarProps={{
          searchKey: 'name',
          searchPlaceholder: t('Filter by name...'),
          searchDebounceMs: 500,
          additionalSearch: (
            <>
              <Input
                inputMode='text'
                autoComplete='off'
                aria-label={t('Filter by redemption code...')}
                placeholder={t('Filter by redemption code...')}
                value={codeFilter}
                onChange={(event) =>
                  table.getColumn('code')?.setFilterValue(event.target.value)
                }
                className='w-full sm:w-[230px] lg:w-[280px]'
              />
              <Input
                inputMode='numeric'
                autoComplete='off'
                aria-label={t('Filter by ID...')}
                placeholder={t('Filter by ID...')}
                value={idFilter}
                onChange={(event) =>
                  table.getColumn('id')?.setFilterValue(event.target.value)
                }
                className='w-full sm:w-[130px] lg:w-[150px]'
              />
            </>
          ),
          filters: [
            {
              columnId: 'status',
              title: t('Status'),
              options: redemptionStatusOptions,
              singleSelect: true,
            },
          ],
        }}
        mobile={
          <RedemptionsMobileList
            table={table}
            isLoading={isLoading}
            onUserClick={handleUserClick}
          />
        }
        getRowClassName={(row, { isMobile }) => {
          if (!isDisabledRedemptionRow(row.original)) return undefined
          return isMobile ? DISABLED_ROW_MOBILE : DISABLED_ROW_DESKTOP
        }}
        bulkActions={<DataTableBulkActions table={table} />}
      />
      <UserInfoDialog
        userId={selectedUserId}
        open={userInfoDialogOpen}
        onOpenChange={setUserInfoDialogOpen}
      />
    </>
  )
}

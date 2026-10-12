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
import {
  keepPreviousData,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import {
  CalendarDays,
  Download,
  Loader2,
  Plus,
  RefreshCw,
  Search,
  Wrench,
} from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useDebounce } from '@/hooks/use-debounce'
import { toIntlLocale } from '@/i18n/languages'
import { getUserFacingErrorMessage } from '@/lib/user-facing-error'
import { cn } from '@/lib/utils'

import {
  downloadInvoiceFile,
  fetchInvoiceConfig,
  fetchInvoiceRequest,
  fetchInvoiceRequests,
  invoiceQueryKeys,
  type InvoiceListParams,
} from '../api'
import {
  buildInvoiceListSearch,
  INVOICE_PAGE_SIZES,
  resolveInvoiceListState,
  type InvoiceListSearch,
  type InvoiceListState,
} from '../lib/invoice-list-search'
import {
  formatInvoiceAmount,
  formatInvoiceTimestamp,
  INVOICE_STATUS_META,
  isInvoiceRequestExpiring,
} from '../lib/invoice-utils'
import {
  INVOICE_STATUS,
  INVOICE_STATUS_OPTIONS,
  type InvoiceRequest,
  type InvoiceStatus,
} from '../types'
import { InvoiceMaintenanceDialog } from './invoice-maintenance-dialog'
import { InvoiceRequestDialog } from './invoice-request-dialog'

type InvoiceListPageProps = {
  admin?: boolean
  search: InvoiceListSearch
  onSearchChange: (search: InvoiceListSearch) => void
}

export function InvoiceListPage(props: InvoiceListPageProps) {
  const admin = props.admin ?? false
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const state = resolveInvoiceListState(props.search, admin)
  const [keywordInput, setKeywordInput] = useState(state.keyword)
  const [createOpen, setCreateOpen] = useState(false)
  const [maintenanceOpen, setMaintenanceOpen] = useState(false)
  const [downloadingId, setDownloadingId] = useState<number | null>(null)
  const debouncedKeyword = useDebounce(keywordInput, 300)
  const onSearchChange = props.onSearchChange
  const currentSearch = props.search

  const params: InvoiceListParams = {
    page: state.page,
    pageSize: state.pageSize,
    status: state.status === 'all' ? undefined : state.status,
    keyword: admin ? state.keyword || undefined : undefined,
  }
  // The query key follows the URL state, so a filter change that also resets
  // the page issues exactly one request and stale responses are discarded.
  const listQuery = useQuery({
    queryKey: invoiceQueryKeys.list(admin, params),
    queryFn: () => fetchInvoiceRequests(params, admin),
    placeholderData: keepPreviousData,
    retry: false,
    meta: { errorToast: false },
  })
  const configQuery = useQuery({
    queryKey: invoiceQueryKeys.config(),
    queryFn: fetchInvoiceConfig,
    enabled: !admin,
    retry: false,
    meta: { errorToast: false },
  })

  useEffect(() => {
    const keyword = debouncedKeyword.trim()
    if (!admin || keyword === (currentSearch.keyword ?? '')) return
    onSearchChange({
      ...currentSearch,
      page: undefined,
      keyword: keyword || undefined,
    })
  }, [admin, currentSearch, debouncedKeyword, onSearchChange])

  const updateList = (patch: Partial<InvoiceListState>) =>
    onSearchChange(
      buildInvoiceListSearch({ ...state, page: 1, ...patch }, admin)
    )

  const downloadIssuedInvoice = async (invoice: InvoiceRequest) => {
    if (downloadingId !== null) return
    setDownloadingId(invoice.id)
    try {
      const detail = await queryClient.fetchQuery({
        queryKey: invoiceQueryKeys.detail(false, invoice.id),
        queryFn: () => fetchInvoiceRequest(invoice.id, false),
        retry: false,
        meta: { errorToast: false },
      })
      if (detail.files.length === 1) {
        await downloadInvoiceFile(invoice.id, detail.files[0])
        return
      }
      void navigate({
        to: '/invoices/$id',
        params: { id: String(invoice.id) },
        hash: 'invoice-files',
      })
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setDownloadingId(null)
    }
  }

  const total = listQuery.data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / state.pageSize))
  const loading = listQuery.isFetching
  let listContent: React.ReactNode
  if (listQuery.isError) {
    listContent = (
      <ErrorState
        className='bg-card rounded-xl border'
        title={t('Failed to load invoice applications')}
        description={getUserFacingErrorMessage(listQuery.error)}
        onRetry={() => void listQuery.refetch()}
      />
    )
  } else {
    listContent = (
      <InvoiceTable
        items={listQuery.data?.items ?? []}
        loading={listQuery.isPending}
        admin={admin}
        locale={locale}
        downloadingId={downloadingId}
        onDownload={(invoice) => void downloadIssuedInvoice(invoice)}
        onSelect={(invoice) =>
          navigate({
            to: `${admin ? '/admin-invoices' : '/invoices'}/${invoice.id}`,
          })
        }
      />
    )
  }

  return (
    <div className='h-full overflow-y-auto px-4 py-6 sm:px-8'>
      <div className='mx-auto w-full max-w-[1440px] space-y-5'>
        <header>
          <h1 className='text-xl font-semibold tracking-tight'>
            {admin ? t('Invoice Management') : t('Invoice Applications')}
          </h1>
          <p className='text-muted-foreground mt-1 text-sm'>
            {admin
              ? t(
                  'Review invoice information, upload issued files, and update application status'
                )
              : t(
                  'Select paid top-up orders, submit invoice information, and download issued invoices'
                )}
          </p>
        </header>
        {!admin && configQuery.data && (
          <div className='border-primary/25 bg-primary/5 flex items-start gap-3 rounded-xl border px-4 py-3 text-sm'>
            <CalendarDays
              className='text-primary mt-0.5 h-4 w-4 shrink-0'
              aria-hidden='true'
            />
            <p className='leading-6'>
              {t(
                'Invoices are issued together on day {{day}} of each month. Please confirm all invoice information before submitting; completed electronic invoices can be downloaded from the application details.',
                { day: configQuery.data.issue_day }
              )}
            </p>
          </div>
        )}
        <div className='bg-card flex flex-wrap items-center gap-2 rounded-xl border p-3'>
          {!admin && (
            <Button onClick={() => setCreateOpen(true)}>
              <Plus className='size-4' />
              {t('Request invoice')}
            </Button>
          )}
          <Button variant='outline' onClick={() => void listQuery.refetch()}>
            <RefreshCw className={cn('size-4', loading && 'animate-spin')} />
            {t('Refresh')}
          </Button>
          {admin && (
            <Button variant='outline' onClick={() => setMaintenanceOpen(true)}>
              <Wrench className='size-4' />
              {t('Maintenance')}
            </Button>
          )}
          <Label htmlFor='invoice-status-filter' className='sr-only'>
            {t('Filter by application status')}
          </Label>
          <Select
            value={String(state.status)}
            onValueChange={(value) => {
              if (value === null) return
              updateList({
                status:
                  value === 'all' ? 'all' : (Number(value) as InvoiceStatus),
              })
            }}
          >
            <SelectTrigger id='invoice-status-filter' className='w-[150px]'>
              <SelectValue>
                {state.status === 'all'
                  ? t('All statuses')
                  : t(INVOICE_STATUS_META[state.status].labelKey)}
              </SelectValue>
            </SelectTrigger>
            <SelectContent
              side='bottom'
              align='start'
              sideOffset={6}
              alignItemWithTrigger={false}
            >
              <SelectItem value='all'>{t('All statuses')}</SelectItem>
              {INVOICE_STATUS_OPTIONS.map((value) => (
                <SelectItem key={value} value={String(value)}>
                  {t(INVOICE_STATUS_META[value].labelKey)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {admin && (
            <div className='relative min-w-[280px] flex-1 md:max-w-[520px]'>
              <Label htmlFor='invoice-search' className='sr-only'>
                {t(
                  'Search by order number, username, invoice title, or tax number'
                )}
              </Label>
              <Search
                className='text-muted-foreground pointer-events-none absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2'
                aria-hidden='true'
              />
              <Input
                id='invoice-search'
                value={keywordInput}
                onChange={(event) => setKeywordInput(event.target.value)}
                placeholder={t(
                  'Search by order number, username, invoice title, or tax number'
                )}
                className='pl-9'
              />
            </div>
          )}
        </div>

        {listContent}
        <div className='flex flex-wrap items-center justify-between gap-3'>
          <span className='text-muted-foreground text-xs'>
            {t('Total {{n}} records', { n: total })}
          </span>
          <div className='flex items-center gap-2'>
            <Label htmlFor='invoice-page-size' className='sr-only'>
              {t('Records per page')}
            </Label>
            <Select
              value={String(state.pageSize)}
              onValueChange={(value) => {
                if (value !== null) updateList({ pageSize: Number(value) })
              }}
            >
              <SelectTrigger id='invoice-page-size' className='w-[100px]'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {INVOICE_PAGE_SIZES.map((size) => (
                  <SelectItem key={size} value={String(size)}>
                    {t('{{n}} / page', { n: size })}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button
              variant='outline'
              size='sm'
              disabled={loading || state.page <= 1}
              onClick={() => updateList({ page: Math.max(1, state.page - 1) })}
            >
              {t('Previous page')}
            </Button>
            <span className='text-muted-foreground text-xs'>
              {t('Page {{p}} / {{total}}', {
                p: state.page,
                total: totalPages,
              })}
            </span>
            <Button
              variant='outline'
              size='sm'
              disabled={loading || state.page >= totalPages}
              onClick={() =>
                updateList({ page: Math.min(totalPages, state.page + 1) })
              }
            >
              {t('Next page')}
            </Button>
          </div>
        </div>
      </div>
      {!admin && (
        <InvoiceRequestDialog
          open={createOpen}
          onOpenChange={setCreateOpen}
          onCreated={() =>
            void queryClient.invalidateQueries({
              queryKey: invoiceQueryKeys.all,
            })
          }
        />
      )}
      {admin && (
        <InvoiceMaintenanceDialog
          open={maintenanceOpen}
          onOpenChange={setMaintenanceOpen}
        />
      )}
    </div>
  )
}

type InvoiceTableProps = {
  items: InvoiceRequest[]
  loading: boolean
  admin: boolean
  locale?: string
  downloadingId: number | null
  onDownload: (invoice: InvoiceRequest) => void
  onSelect: (invoice: InvoiceRequest) => void
}

function InvoiceTable(props: InvoiceTableProps) {
  const { t } = useTranslation()
  // Users get a quick download column; administrators get the user column.
  const columns = 8
  const skeletonRows = ['one', 'two', 'three', 'four', 'five']
  const skeletonColumns = [
    'id',
    'user',
    'company',
    'tax',
    'amount',
    'status',
    'submitted',
    'updated',
  ]
  let tableContent: React.ReactNode
  if (props.loading && props.items.length === 0) {
    tableContent = skeletonRows.map((row) => (
      <TableRow key={row}>
        {skeletonColumns.map((column) => (
          <TableCell key={column}>
            <Skeleton className='h-4 w-full' />
          </TableCell>
        ))}
      </TableRow>
    ))
  } else if (props.items.length === 0) {
    tableContent = (
      <TableRow>
        <TableCell
          colSpan={columns}
          className='text-muted-foreground py-12 text-center'
        >
          {t('No invoice applications')}
        </TableCell>
      </TableRow>
    )
  } else {
    tableContent = props.items.map((invoice) => {
      const meta = INVOICE_STATUS_META[invoice.status]
      const expiring = isInvoiceRequestExpiring(invoice)
      const downloadable =
        invoice.status === INVOICE_STATUS.ISSUED && invoice.redacted_time === 0
      return (
        <TableRow
          key={invoice.id}
          className='hover:bg-accent/40 focus-visible:ring-ring cursor-pointer focus-visible:ring-2 focus-visible:outline-none'
          role='link'
          tabIndex={0}
          onClick={() => props.onSelect(invoice)}
          onKeyDown={(event) => {
            if (event.key === 'Enter' || event.key === ' ') {
              event.preventDefault()
              props.onSelect(invoice)
            }
          }}
        >
          <TableCell className='font-mono'>{invoice.id}</TableCell>
          {props.admin && (
            <TableCell>{invoice.username || invoice.user_id}</TableCell>
          )}
          <TableCell className='max-w-[260px] truncate'>
            {invoice.redacted_time > 0
              ? t('Archived invoice')
              : invoice.company_name}
          </TableCell>
          <TableCell className='font-mono text-xs'>
            {invoice.tax_number}
          </TableCell>
          <TableCell className='text-right font-semibold'>
            {formatInvoiceAmount(invoice.total_money)}
          </TableCell>
          <TableCell>
            <div className='flex flex-wrap gap-1.5'>
              <Badge
                variant='outline'
                className={cn('font-normal', meta.badgeClass)}
              >
                {t(meta.labelKey)}
              </Badge>
              {expiring && (
                <Badge
                  variant='outline'
                  className='border-orange-500/40 bg-orange-500/10 text-orange-600 dark:text-orange-400'
                >
                  {t('Expires soon')}
                </Badge>
              )}
            </div>
          </TableCell>
          <TableCell className='text-muted-foreground text-xs'>
            {formatInvoiceTimestamp(invoice.created_time, props.locale)}
          </TableCell>
          <TableCell className='text-muted-foreground text-xs'>
            {formatInvoiceTimestamp(invoice.updated_time, props.locale)}
          </TableCell>
          {!props.admin && (
            <TableCell className='text-right'>
              {downloadable && (
                <Button
                  variant='outline'
                  size='sm'
                  disabled={props.downloadingId !== null}
                  aria-label={t('Download invoice for application #{{id}}', {
                    id: invoice.id,
                  })}
                  onClick={(event) => {
                    event.stopPropagation()
                    props.onDownload(invoice)
                  }}
                  onKeyDown={(event) => event.stopPropagation()}
                >
                  {props.downloadingId === invoice.id ? (
                    <Loader2 className='size-4 animate-spin' />
                  ) : (
                    <Download className='size-4' />
                  )}
                  {t('Download')}
                </Button>
              )}
            </TableCell>
          )}
        </TableRow>
      )
    })
  }
  return (
    <div className='bg-card overflow-x-auto rounded-xl border'>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className='w-16'>ID</TableHead>
            {props.admin && <TableHead>{t('User')}</TableHead>}
            <TableHead>{t('Invoice title')}</TableHead>
            <TableHead>{t('Tax number')}</TableHead>
            <TableHead className='text-right'>{t('Amount')}</TableHead>
            <TableHead>{t('Status')}</TableHead>
            <TableHead>{t('Submitted at')}</TableHead>
            <TableHead>{t('Updated')}</TableHead>
            {!props.admin && (
              <TableHead className='text-right'>{t('Invoice file')}</TableHead>
            )}
          </TableRow>
        </TableHeader>
        <TableBody>{tableContent}</TableBody>
      </Table>
    </div>
  )
}

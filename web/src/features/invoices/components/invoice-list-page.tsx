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
import { useNavigate } from '@tanstack/react-router'
import { CalendarDays, Plus, RefreshCw, Search, Wrench } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

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
import { getUserFacingErrorMessage } from '@/lib/user-facing-error'
import { cn } from '@/lib/utils'

import { fetchInvoiceRequests } from '../api'
import {
  formatInvoiceTimestamp,
  INVOICE_STATUS_META,
  isInvoiceRequestExpiring,
} from '../lib/invoice-utils'
import {
  INVOICE_STATUS_OPTIONS,
  type InvoiceRequest,
  type InvoiceStatus,
} from '../types'
import { InvoiceMaintenanceDialog } from './invoice-maintenance-dialog'
import { InvoiceRequestDialog } from './invoice-request-dialog'

const PAGE_SIZES = [10, 20, 50, 100]

export function InvoiceListPage({ admin = false }: { admin?: boolean }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [items, setItems] = useState<InvoiceRequest[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [status, setStatus] = useState<'all' | InvoiceStatus>('all')
  const [keyword, setKeyword] = useState('')
  const [loading, setLoading] = useState(true)
  const [createOpen, setCreateOpen] = useState(false)
  const [maintenanceOpen, setMaintenanceOpen] = useState(false)
  const debouncedKeyword = useDebounce(keyword, 300)
  const params = useMemo(
    () => ({
      page,
      pageSize,
      status: status === 'all' ? undefined : status,
      keyword: admin ? debouncedKeyword.trim() || undefined : undefined,
    }),
    [admin, debouncedKeyword, page, pageSize, status]
  )

  const refresh = async () => {
    setLoading(true)
    try {
      const result = await fetchInvoiceRequests(params, admin)
      setItems(result.items || [])
      setTotal(result.total || 0)
    } catch (error) {
      setItems([])
      setTotal(0)
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void refresh()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [params])
  useEffect(() => setPage(1), [status, pageSize, debouncedKeyword])

  const totalPages = Math.max(1, Math.ceil(total / pageSize))
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
        {!admin && (
          <div className='border-primary/25 bg-primary/5 flex items-start gap-3 rounded-xl border px-4 py-3 text-sm'>
            <CalendarDays
              className='text-primary mt-0.5 h-4 w-4 shrink-0'
              aria-hidden='true'
            />
            <p className='leading-6'>
              {t(
                'Invoices are issued together on the 10th of each month. Please confirm all invoice information before submitting; completed electronic invoices can be downloaded from the application details.'
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
          <Button variant='outline' onClick={refresh}>
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
            value={String(status)}
            onValueChange={(value) =>
              setStatus(
                value === 'all' ? 'all' : (Number(value) as InvoiceStatus)
              )
            }
          >
            <SelectTrigger id='invoice-status-filter' className='w-[150px]'>
              <SelectValue>
                {status === 'all'
                  ? t('All statuses')
                  : t(INVOICE_STATUS_META[status].labelKey)}
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
                  'Search by order number, username, company name, or tax number'
                )}
              </Label>
              <Search
                className='text-muted-foreground pointer-events-none absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2'
                aria-hidden='true'
              />
              <Input
                id='invoice-search'
                value={keyword}
                onChange={(event) => setKeyword(event.target.value)}
                placeholder={t(
                  'Search by order number, username, company name, or tax number'
                )}
                className='pl-9'
              />
            </div>
          )}
        </div>

        <InvoiceTable
          items={items}
          loading={loading}
          admin={admin}
          onSelect={(invoice) =>
            navigate({
              to: `${admin ? '/admin-invoices' : '/invoices'}/${invoice.id}`,
            })
          }
        />
        <div className='flex flex-wrap items-center justify-between gap-3'>
          <span className='text-muted-foreground text-xs'>
            {t('Total {{n}} records', { n: total })}
          </span>
          <div className='flex items-center gap-2'>
            <Label htmlFor='invoice-page-size' className='sr-only'>
              {t('Records per page')}
            </Label>
            <Select
              value={String(pageSize)}
              onValueChange={(value) => setPageSize(Number(value))}
            >
              <SelectTrigger id='invoice-page-size' className='w-[100px]'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {PAGE_SIZES.map((size) => (
                  <SelectItem key={size} value={String(size)}>
                    {t('{{n}} / page', { n: size })}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button
              variant='outline'
              size='sm'
              disabled={loading || page <= 1}
              onClick={() => setPage((value) => Math.max(1, value - 1))}
            >
              {t('Previous')}
            </Button>
            <span className='text-muted-foreground text-xs'>
              {t('Page {{p}} / {{total}}', { p: page, total: totalPages })}
            </span>
            <Button
              variant='outline'
              size='sm'
              disabled={loading || page >= totalPages}
              onClick={() =>
                setPage((value) => Math.min(totalPages, value + 1))
              }
            >
              {t('Next')}
            </Button>
          </div>
        </div>
      </div>
      {!admin && (
        <InvoiceRequestDialog
          open={createOpen}
          onOpenChange={setCreateOpen}
          onCreated={refresh}
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

function InvoiceTable({
  items,
  loading,
  admin,
  onSelect,
}: {
  items: InvoiceRequest[]
  loading: boolean
  admin: boolean
  onSelect: (invoice: InvoiceRequest) => void
}) {
  const { t } = useTranslation()
  const columns = admin ? 8 : 7
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
  ].slice(0, columns)
  let tableContent: React.ReactNode
  if (loading && items.length === 0) {
    tableContent = skeletonRows.map((row) => (
      <TableRow key={row}>
        {skeletonColumns.map((column) => (
          <TableCell key={column}>
            <Skeleton className='h-4 w-full' />
          </TableCell>
        ))}
      </TableRow>
    ))
  } else if (items.length === 0) {
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
    tableContent = items.map((invoice) => {
      const meta = INVOICE_STATUS_META[invoice.status]
      const expiring = isInvoiceRequestExpiring(invoice)
      return (
        <TableRow
          key={invoice.id}
          className='hover:bg-accent/40 focus-visible:ring-ring cursor-pointer focus-visible:ring-2 focus-visible:outline-none'
          role='link'
          tabIndex={0}
          onClick={() => onSelect(invoice)}
          onKeyDown={(event) => {
            if (event.key === 'Enter' || event.key === ' ') {
              event.preventDefault()
              onSelect(invoice)
            }
          }}
        >
          <TableCell className='font-mono'>{invoice.id}</TableCell>
          {admin && (
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
            ¥{Number(invoice.total_money || 0).toFixed(2)}
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
            {formatInvoiceTimestamp(invoice.created_time)}
          </TableCell>
          <TableCell className='text-muted-foreground text-xs'>
            {formatInvoiceTimestamp(invoice.updated_time)}
          </TableCell>
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
            {admin && <TableHead>{t('User')}</TableHead>}
            <TableHead>{t('Company name')}</TableHead>
            <TableHead>{t('Tax number')}</TableHead>
            <TableHead className='text-right'>{t('Amount')}</TableHead>
            <TableHead>{t('Status')}</TableHead>
            <TableHead>{t('Submitted at')}</TableHead>
            <TableHead>{t('Updated')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>{tableContent}</TableBody>
      </Table>
    </div>
  )
}

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
  Loader2,
  RefreshCw,
  RotateCcw,
  ShieldCheck,
  Trash2,
} from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { getUserFacingErrorMessage } from '@/lib/user-facing-error'

import {
  cleanupInvoiceOrphans,
  fetchInvoiceMaintenance,
  reconcileInvoiceStorage,
  retryInvoiceCleanup,
  retryInvoiceNotification,
} from '../api'
import { formatInvoiceTimestamp } from '../lib/invoice-utils'
import type {
  InvoiceMaintenance,
  InvoiceStorageReconcileReport,
} from '../types'

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function InvoiceMaintenanceDialog({ open, onOpenChange }: Props) {
  const { t } = useTranslation()
  const [data, setData] = useState<InvoiceMaintenance | null>(null)
  const [report, setReport] = useState<InvoiceStorageReconcileReport | null>(
    null
  )
  const [busy, setBusy] = useState('')
  const [cleanupConfirmation, setCleanupConfirmation] = useState<
    'orphans' | null
  >(null)

  const refresh = useCallback(async () => {
    setBusy('refresh')
    try {
      setData(await fetchInvoiceMaintenance())
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setBusy('')
    }
  }, [])

  useEffect(() => {
    if (open) void refresh()
  }, [open, refresh])

  const run = async (key: string, action: () => Promise<void>) => {
    setBusy(key)
    try {
      await action()
      toast.success(t('Operation queued successfully'))
      await refresh()
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
      setBusy('')
    }
  }

  const reconcile = async () => {
    setBusy('reconcile')
    try {
      const next = await reconcileInvoiceStorage()
      setReport(next)
      toast.success(t('Storage reconciliation completed'))
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setBusy('')
    }
  }

  const confirmCleanup = async () => {
    const target = cleanupConfirmation
    setCleanupConfirmation(null)
    if (target === 'orphans' && report) {
      await run('orphans', () => cleanupInvoiceOrphans(report.orphan_keys))
    }
  }

  let cleanupSummary = t('Are you sure?')
  if (cleanupConfirmation === 'orphans' && report) {
    cleanupSummary = t(
      '{{objects}} orphan objects, {{missing}} missing files',
      {
        objects: report.orphan_keys.length,
        missing: report.missing_files.length,
      }
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className='flex max-h-[90dvh] flex-col sm:max-w-5xl'
        closeLabel={t('Close')}
      >
        <DialogHeader>
          <DialogTitle>{t('Invoice maintenance')}</DialogTitle>
          <DialogDescription>
            {t(
              'Inspect file cleanup, notification delivery, and storage consistency.'
            )}
          </DialogDescription>
        </DialogHeader>
        <div className='min-h-0 flex-1 space-y-5 overflow-y-auto pr-1'>
          <div className='flex flex-wrap items-center gap-2'>
            <Button
              variant='outline'
              onClick={refresh}
              disabled={Boolean(busy)}
            >
              {busy === 'refresh' ? (
                <Loader2 className='size-4 animate-spin' />
              ) : (
                <RefreshCw className='size-4' />
              )}
              {t('Refresh')}
            </Button>
            <Button onClick={reconcile} disabled={Boolean(busy)}>
              {busy === 'reconcile' ? (
                <Loader2 className='size-4 animate-spin' />
              ) : (
                <ShieldCheck className='size-4' />
              )}
              {t('Reconcile storage')}
            </Button>
          </div>

          {data ? (
            <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-4'>
              <SummaryCard
                label={t('Storage profiles')}
                value={data.profiles.length}
              />
              <SummaryCard
                label={t('Uploads in progress')}
                value={data.uploads.length}
              />
              <SummaryCard
                label={t('Pending file cleanups')}
                value={data.cleanups.length}
              />
              <SummaryCard
                label={t('Pending notifications')}
                value={data.notifications.length}
              />
            </div>
          ) : (
            <div className='text-muted-foreground rounded-xl border p-8 text-center text-sm'>
              {busy ? t('Loading...') : t('No maintenance data')}
            </div>
          )}

          {report && (
            <section className='space-y-3 rounded-xl border p-4'>
              <div className='flex flex-wrap items-center justify-between gap-2'>
                <div>
                  <h3 className='font-semibold'>
                    {t('Storage reconciliation')}
                  </h3>
                  <p className='text-muted-foreground text-xs'>
                    {t(
                      '{{objects}} orphan objects, {{missing}} missing files',
                      {
                        objects: report.orphan_keys.length,
                        missing: report.missing_files.length,
                      }
                    )}
                  </p>
                </div>
                {report.orphan_keys.length > 0 && (
                  <Button
                    variant='destructive'
                    disabled={Boolean(busy)}
                    onClick={() => setCleanupConfirmation('orphans')}
                  >
                    <Trash2 className='size-4' />
                    {t('Queue orphan cleanup')}
                  </Button>
                )}
              </div>
              {report.profiles.some((profile) => profile.error) && (
                <div className='border-destructive/30 bg-destructive/5 space-y-1 rounded-lg border p-3 text-xs'>
                  {report.profiles
                    .filter((profile) => profile.error)
                    .map((profile) => (
                      <p key={profile.storage_profile_id}>
                        {profile.storage_type} #{profile.storage_profile_id}:{' '}
                        {profile.error}
                      </p>
                    ))}
                </div>
              )}
              {report.missing_files.length > 0 && (
                <div className='rounded-lg border border-amber-500/30 bg-amber-500/5 p-3 text-xs'>
                  {t(
                    'Missing files remain in the database and require manual recovery from the referenced storage profile.'
                  )}
                </div>
              )}
            </section>
          )}

          {data && data.cleanups.length > 0 && (
            <MaintenanceSection title={t('File cleanup queue')}>
              {data.cleanups.map((item) => (
                <MaintenanceRow
                  key={item.id}
                  title={`${item.storage_type} · ${item.storage_key}`}
                  detail={
                    item.last_error ||
                    t('Next attempt: {{time}}', {
                      time: formatInvoiceTimestamp(item.next_attempt_time),
                    })
                  }
                  badge={t('{{n}} attempts', { n: item.attempts })}
                  action={
                    <Button
                      size='sm'
                      variant='outline'
                      disabled={Boolean(busy)}
                      onClick={() =>
                        void run(`cleanup-${item.id}`, () =>
                          retryInvoiceCleanup(item.id)
                        )
                      }
                    >
                      <RotateCcw className='size-4' />
                      {t('Retry')}
                    </Button>
                  }
                />
              ))}
            </MaintenanceSection>
          )}

          {data && data.notifications.length > 0 && (
            <MaintenanceSection title={t('Notification delivery queue')}>
              {data.notifications.map((item) => (
                <MaintenanceRow
                  key={item.id}
                  title={`${item.kind} · #${item.invoice_request_id}`}
                  detail={
                    item.last_error ||
                    item.recipient ||
                    t('Waiting for delivery')
                  }
                  badge={t('{{n}} attempts', { n: item.attempts })}
                  action={
                    <Button
                      size='sm'
                      variant='outline'
                      disabled={Boolean(busy)}
                      onClick={() =>
                        void run(`notification-${item.id}`, () =>
                          retryInvoiceNotification(item.id)
                        )
                      }
                    >
                      <RotateCcw className='size-4' />
                      {t('Retry')}
                    </Button>
                  }
                />
              ))}
            </MaintenanceSection>
          )}
        </div>
      </DialogContent>
      <AlertDialog
        open={cleanupConfirmation !== null}
        onOpenChange={(nextOpen) => {
          if (!nextOpen) setCleanupConfirmation(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Confirm Cleanup')}</AlertDialogTitle>
            <AlertDialogDescription>
              {cleanupSummary} {t('This action cannot be undone.')}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('Cancel')}</AlertDialogCancel>
            <AlertDialogAction
              variant='destructive'
              onClick={() => void confirmCleanup()}
            >
              {t('Confirm Cleanup')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Dialog>
  )
}

function SummaryCard({ label, value }: { label: string; value: number }) {
  return (
    <div className='bg-card rounded-xl border p-3'>
      <div className='text-muted-foreground text-xs'>{label}</div>
      <div className='mt-1 text-xl font-semibold tabular-nums'>{value}</div>
    </div>
  )
}

function MaintenanceSection({
  title,
  children,
}: {
  title: string
  children: React.ReactNode
}) {
  return (
    <section className='space-y-2'>
      <h3 className='font-semibold'>{title}</h3>
      <div className='divide-y overflow-hidden rounded-xl border'>
        {children}
      </div>
    </section>
  )
}

function MaintenanceRow({
  title,
  detail,
  badge,
  action,
}: {
  title: string
  detail: string
  badge: string
  action: React.ReactNode
}) {
  return (
    <div className='flex flex-wrap items-center gap-3 p-3'>
      <div className='min-w-0 flex-1'>
        <div className='truncate text-sm font-medium'>{title}</div>
        <div className='text-muted-foreground mt-1 line-clamp-2 text-xs'>
          {detail}
        </div>
      </div>
      <Badge variant='outline'>{badge}</Badge>
      {action}
    </div>
  )
}

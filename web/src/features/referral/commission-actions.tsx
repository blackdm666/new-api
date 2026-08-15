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
  Calendar03Icon,
  CoinsDollarIcon,
  Database01Icon,
  InformationCircleIcon,
  Wallet02Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
  InputGroupText,
} from '@/components/ui/input-group'
import { Label } from '@/components/ui/label'
import { getSelf } from '@/lib/api'
import { formatDateStr, formatQuota } from '@/lib/format'
import { getUserFacingErrorMessage } from '@/lib/user-facing-error'
import { useAuthStore } from '@/stores/auth-store'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { createAffiliatePayout, transferAffiliateCommission } from './api'
import { formatCents } from './lib'

type CommissionActionsProps = {
  availableCents: number
  minimumPayoutCents: number
  settlementDay: number
  loading: boolean
  onTransfer: () => void
  onPayout: () => void
}

export function CommissionActions(props: CommissionActionsProps) {
  const { t } = useTranslation()
  const canTransfer = props.availableCents >= 100
  const canPayout = props.availableCents >= props.minimumPayoutCents

  return (
    <Card data-card-hover='false' className='bg-muted/20 gap-0 py-0'>
      <CardContent className='grid p-0 lg:grid-cols-[minmax(140px,0.38fr)_minmax(360px,1fr)_minmax(360px,1fr)] lg:items-stretch'>
        <div className='flex items-center px-4 py-4 sm:px-5'>
          <h2 className='text-base font-semibold'>{t('Commission use')}</h2>
        </div>
        <div className='border-border/70 flex flex-wrap items-center gap-3 border-t px-4 py-3 lg:border-t-0 lg:border-l'>
          <Button
            type='button'
            className='h-10 shrink-0 px-4'
            disabled={props.loading || !canTransfer}
            onClick={props.onTransfer}
          >
            <HugeiconsIcon icon={Database01Icon} strokeWidth={2} />
            {t('Transfer to API balance')}
          </Button>
          <span className='text-muted-foreground text-sm'>
            {t('Instant credit · no referral commission')}
          </span>
        </div>
        <div className='border-border/70 flex flex-wrap items-center gap-3 border-t px-4 py-3 lg:border-t-0 lg:border-l'>
          <Button
            type='button'
            variant='outline'
            className='h-10 shrink-0 px-4'
            disabled={props.loading || !canPayout}
            onClick={props.onPayout}
          >
            <HugeiconsIcon icon={Wallet02Icon} strokeWidth={2} />
            {t('Apply for cash payout')}
          </Button>
          <span className='text-muted-foreground text-sm'>
            {t('Minimum {{amount}} · paid on day {{day}} each month', {
              amount: formatCents(props.minimumPayoutCents),
              day: props.settlementDay,
            })}
          </span>
        </div>
      </CardContent>
    </Card>
  )
}

type TransferBalanceDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  availableCents: number
  currentBalanceQuota: number
}

export function TransferBalanceDialog(props: TransferBalanceDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const setUser = useAuthStore((state) => state.auth.setUser)
  const availableAmount = props.availableCents / 100
  const [amount, setAmount] = useState(String(availableAmount))
  const [submitting, setSubmitting] = useState(false)
  const amountCents = useMemo(
    () => Math.round(Number.parseFloat(amount || '0') * 100),
    [amount]
  )
  const quotaPerUnit =
    useSystemConfigStore.getState().config.currency.quotaPerUnit ||
    DEFAULT_CURRENCY_CONFIG.quotaPerUnit
  const transferQuota = Math.round((amountCents / 100) * quotaPerUnit)
  const valid = amountCents >= 100 && amountCents <= props.availableCents

  useEffect(() => {
    if (props.open) setAmount(String(availableAmount))
  }, [availableAmount, props.open])

  const close = () => {
    if (submitting) return
    props.onOpenChange(false)
  }
  const submit = async () => {
    if (!valid || submitting) return
    setSubmitting(true)
    try {
      await transferAffiliateCommission({
        amount_cents: amountCents,
        request_id: crypto.randomUUID(),
      })
      const self = await getSelf()
      if (self.success && self.data) setUser(self.data)
      await queryClient.invalidateQueries({ queryKey: ['referral'] })
      toast.success(t('Transfer successful'))
      props.onOpenChange(false)
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={props.open} onOpenChange={close}>
      <DialogContent className='sm:max-w-xl' closeLabel={t('Close')}>
        <DialogHeader>
          <DialogTitle className='text-lg font-semibold'>
            {t('Transfer to API balance')}
          </DialogTitle>
          <DialogDescription>
            {t('Move available commission to your API balance instantly.')}
          </DialogDescription>
        </DialogHeader>
        <div className='space-y-4 py-1'>
          <div>
            <p className='text-muted-foreground text-sm'>
              {t('Available commission')}
            </p>
            <p className='mt-1 text-2xl font-semibold tabular-nums'>
              {formatCents(props.availableCents)}
            </p>
          </div>
          <div className='space-y-2'>
            <Label htmlFor='affiliate-transfer-amount'>
              {t('Transfer amount')}
            </Label>
            <InputGroup className='h-11'>
              <InputGroupAddon>
                <InputGroupText>¥</InputGroupText>
              </InputGroupAddon>
              <InputGroupInput
                id='affiliate-transfer-amount'
                type='number'
                min={1}
                max={availableAmount}
                step={0.01}
                value={amount}
                onChange={(event) => setAmount(event.target.value)}
                aria-invalid={amount.length > 0 && !valid}
              />
              <InputGroupAddon align='inline-end'>
                <InputGroupButton
                  type='button'
                  onClick={() => setAmount(String(availableAmount))}
                >
                  {t('Transfer all')}
                </InputGroupButton>
              </InputGroupAddon>
            </InputGroup>
          </div>
          <div className='bg-muted/60 flex gap-3 rounded-lg border p-3 text-sm'>
            <HugeiconsIcon
              icon={InformationCircleIcon}
              strokeWidth={2}
              className='text-primary mt-0.5 size-4 shrink-0'
            />
            <span>
              {t(
                'Converting commission to API balance does not create a top-up order and does not generate referral commission.'
              )}
            </span>
          </div>
          <p className='text-muted-foreground text-sm'>
            {t('API balance after transfer: {{amount}}', {
              amount: formatQuota(
                props.currentBalanceQuota + (valid ? transferQuota : 0)
              ),
            })}
          </p>
        </div>
        <DialogFooter>
          <Button variant='outline' onClick={close} disabled={submitting}>
            {t('Cancel')}
          </Button>
          <Button onClick={() => void submit()} disabled={!valid || submitting}>
            {submitting ? t('Transferring') : t('Confirm transfer')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

type CashPayoutDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  availableCents: number
  minimumCents: number
  nextSettlementTime: number
}

export function CashPayoutDialog(props: CashPayoutDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [amount, setAmount] = useState(String(props.minimumCents / 100))
  const [accountName, setAccountName] = useState('')
  const [account, setAccount] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const amountCents = useMemo(
    () => Math.round(Number.parseFloat(amount || '0') * 100),
    [amount]
  )
  const amountValid =
    amountCents >= props.minimumCents && amountCents <= props.availableCents
  const valid =
    amountValid &&
    accountName.trim().length > 0 &&
    accountName.trim().length <= 128 &&
    account.trim().length > 0 &&
    account.trim().length <= 255

  useEffect(() => {
    if (!props.open) return
    setAmount(String(props.minimumCents / 100))
    setAccountName('')
    setAccount('')
  }, [props.minimumCents, props.open])

  const close = () => {
    if (submitting) return
    props.onOpenChange(false)
  }
  const submit = async () => {
    if (!valid || submitting) return
    setSubmitting(true)
    try {
      await createAffiliatePayout({
        request_id: crypto.randomUUID(),
        amount_cents: amountCents,
        payment_method: 'alipay',
        account_name: accountName.trim(),
        account: account.trim(),
      })
      await queryClient.invalidateQueries({ queryKey: ['referral'] })
      toast.success(t('Payout application submitted'))
      props.onOpenChange(false)
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={props.open} onOpenChange={close}>
      <DialogContent className='sm:max-w-lg' closeLabel={t('Close')}>
        <DialogHeader>
          <DialogTitle className='text-lg font-semibold'>
            {t('Apply for cash payout')}
          </DialogTitle>
          <DialogDescription>
            {t(
              'Applications may be submitted at any time. Approved payouts are processed on the 10th of each month.'
            )}
          </DialogDescription>
        </DialogHeader>
        <div className='space-y-4 py-1'>
          <div>
            <p className='text-muted-foreground text-sm'>
              {t('Available for payout')}
            </p>
            <p className='text-primary mt-1 text-2xl font-semibold tabular-nums'>
              {formatCents(props.availableCents)}
            </p>
            <p className='text-muted-foreground mt-1 text-xs'>
              {t('Minimum payout: {{amount}}', {
                amount: formatCents(props.minimumCents),
              })}
            </p>
          </div>
          <div className='space-y-2'>
            <Label htmlFor='affiliate-payout-amount'>
              {t('Payout amount')}
            </Label>
            <InputGroup className='h-11'>
              <InputGroupAddon>
                <InputGroupText>¥</InputGroupText>
              </InputGroupAddon>
              <InputGroupInput
                id='affiliate-payout-amount'
                type='number'
                min={props.minimumCents / 100}
                max={props.availableCents / 100}
                step='0.01'
                value={amount}
                onChange={(event) => setAmount(event.target.value)}
                aria-invalid={amount.length > 0 && !amountValid}
              />
              <InputGroupAddon align='inline-end'>
                <InputGroupButton
                  type='button'
                  onClick={() => setAmount(String(props.availableCents / 100))}
                >
                  {t('Payout all')}
                </InputGroupButton>
              </InputGroupAddon>
            </InputGroup>
          </div>
          <div className='space-y-2'>
            <Label htmlFor='affiliate-payout-account-name'>
              {t('Account name')}
            </Label>
            <Input
              id='affiliate-payout-account-name'
              value={accountName}
              onChange={(event) => setAccountName(event.target.value)}
              maxLength={128}
              autoComplete='name'
            />
            <p className='text-muted-foreground text-xs'>
              {t('The recipient name must match Alipay identity verification.')}
            </p>
          </div>
          <div className='space-y-2'>
            <Label htmlFor='affiliate-payout-account'>
              {t('Alipay account')}
            </Label>
            <Input
              id='affiliate-payout-account'
              value={account}
              onChange={(event) => setAccount(event.target.value)}
              maxLength={255}
              placeholder={t('Mobile number or email')}
              autoComplete='username'
            />
            <p className='text-muted-foreground text-xs'>
              {t('Alipay accounts support mobile numbers or email addresses.')}
            </p>
          </div>
          <div className='bg-muted/60 flex items-center justify-between gap-3 rounded-lg border px-3 py-2.5 text-sm'>
            <span className='flex items-center gap-2'>
              <HugeiconsIcon
                icon={Calendar03Icon}
                strokeWidth={2}
                className='size-4'
              />
              {t('Scheduled payout date')}
            </span>
            <span className='font-medium tabular-nums'>
              {formatDateStr(new Date(props.nextSettlementTime * 1000))}
            </span>
          </div>
          <div className='flex gap-3 text-sm'>
            <HugeiconsIcon
              icon={CoinsDollarIcon}
              strokeWidth={2}
              className='text-primary mt-0.5 size-4 shrink-0'
            />
            <span className='text-muted-foreground'>
              {t(
                'The requested amount is reserved immediately and cannot be converted to API balance while the application is pending.'
              )}
            </span>
          </div>
        </div>
        <DialogFooter>
          <Button variant='outline' onClick={close} disabled={submitting}>
            {t('Cancel')}
          </Button>
          <Button disabled={!valid || submitting} onClick={() => void submit()}>
            {submitting ? t('Submitting') : t('Submit application')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

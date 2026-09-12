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
import { Copy, KeyRound } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { formatTimestampToDate } from '@/lib/format'
import { getUserFacingErrorMessage } from '@/lib/user-facing-error'

import {
  getEmailReceiptEndpoint,
  rotateEmailReceiptEndpointToken,
  updateEmailReceiptEndpoint,
} from '../api'
import {
  SettingsControlGroup,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import {
  SMTPProfileStatusCard,
  type SMTPProfileState,
} from './smtp-profile-status-card'

export function EmailReceiptSettings() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [revealedToken, setRevealedToken] = useState('')
  const query = useQuery({
    queryKey: ['email-receipt-endpoint'],
    queryFn: getEmailReceiptEndpoint,
    refetchInterval: 15_000,
  })
  const endpoint = query.data?.data
  const updateMutation = useMutation({
    mutationFn: updateEmailReceiptEndpoint,
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: ['email-receipt-endpoint'],
      })
    },
    onError: (error: unknown) => toast.error(getUserFacingErrorMessage(error)),
  })
  const rotateMutation = useMutation({
    mutationFn: rotateEmailReceiptEndpointToken,
    onSuccess: async (response) => {
      setRevealedToken(response.data.token)
      toast.success(t('EventBridge token generated'))
      await queryClient.invalidateQueries({
        queryKey: ['email-receipt-endpoint'],
      })
    },
    onError: (error: unknown) => toast.error(getUserFacingErrorMessage(error)),
  })

  let state: SMTPProfileState = 'disabled'
  if (endpoint?.last_error) {
    state = 'error'
  } else if (endpoint?.enabled && endpoint.last_verified_time > 0) {
    state = 'enabled'
  } else if (endpoint?.enabled) {
    state = 'pending'
  }

  const copy = async (value: string) => {
    await navigator.clipboard.writeText(value)
    toast.success(t('Copied'))
  }

  return (
    <div className='space-y-6'>
      <SMTPProfileStatusCard
        title={t('Delivery receipt interface')}
        description={t(
          'The default provider is Alibaba Cloud Direct Mail through EventBridge event distribution.'
        )}
        state={state}
      />

      <SettingsControlGroup className='space-y-5'>
        <div className='space-y-1'>
          <p className='text-sm font-medium'>
            {t('Alibaba Cloud Direct Mail (EventBridge event distribution)')}
          </p>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Create an event rule on the default cloud-service event bus, select Direct Mail as the event source, and subscribe to delivery success, delivery failure, complaint, unsubscribe, and resubscribe events.'
            )}
          </p>
        </div>

        <div className='space-y-2'>
          <Label htmlFor='email-receipt-url'>{t('Callback URL')}</Label>
          <div className='flex gap-2'>
            <Input
              id='email-receipt-url'
              readOnly
              value={endpoint?.callback_url ?? ''}
            />
            <Button
              type='button'
              variant='outline'
              disabled={!endpoint?.callback_url}
              onClick={() => void copy(endpoint?.callback_url ?? '')}
            >
              <Copy className='size-4' />
              {t('Copy')}
            </Button>
          </div>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Configure this URL as an EventBridge public HTTP target and use the complete event body.'
            )}
          </p>
        </div>

        <div className='space-y-2'>
          <Label htmlFor='email-receipt-token'>{t('EventBridge Token')}</Label>
          <div className='flex gap-2'>
            <Input
              id='email-receipt-token'
              readOnly
              type={revealedToken ? 'text' : 'password'}
              value={
                revealedToken ||
                (endpoint?.token_configured ? 'configured' : '')
              }
              placeholder={t('Generate a token before enabling receipts')}
            />
            {revealedToken ? (
              <Button
                type='button'
                variant='outline'
                onClick={() => void copy(revealedToken)}
              >
                <Copy className='size-4' />
                {t('Copy')}
              </Button>
            ) : null}
            <Button
              type='button'
              variant='outline'
              disabled={rotateMutation.isPending}
              onClick={() => rotateMutation.mutate()}
            >
              <KeyRound className='size-4' />
              {t(
                endpoint?.token_configured ? 'Rotate token' : 'Generate token'
              )}
            </Button>
          </div>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Set this value in the EventBridge HTTP target advanced Token field. It is sent in the x-eventbridge-signature-token header.'
            )}
          </p>
        </div>

        <SettingsSwitchItem>
          <SettingsSwitchContent>
            <Label htmlFor='email-receipt-enabled'>
              {t('Enable delivery receipts')}
            </Label>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Marketing accounts remain unavailable until a valid final delivery receipt is received.'
              )}
            </p>
          </SettingsSwitchContent>
          <Switch
            id='email-receipt-enabled'
            checked={Boolean(endpoint?.enabled)}
            disabled={!endpoint?.token_configured || updateMutation.isPending}
            onCheckedChange={(checked) => updateMutation.mutate(checked)}
          />
        </SettingsSwitchItem>

        <div className='grid gap-3 text-sm sm:grid-cols-2'>
          <div>
            <p className='text-muted-foreground text-xs'>
              {t('Last valid receipt')}
            </p>
            <p className='font-medium'>
              {endpoint?.last_verified_time
                ? formatTimestampToDate(endpoint.last_verified_time)
                : '-'}
            </p>
          </div>
          <div>
            <p className='text-muted-foreground text-xs'>{t('Last error')}</p>
            <p className='font-medium'>{endpoint?.last_error || '-'}</p>
          </div>
        </div>
      </SettingsControlGroup>
    </div>
  )
}

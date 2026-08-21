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
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'

import { SettingsSwitchField } from '../components/settings-form-layout'

export type AntomSettingsValues = {
  AntomEnabled: boolean
  AntomDisplayName: string
  AntomGateway: string
  AntomClientId: string
  AntomMerchantPrivateKey: string
  AntomPublicKey: string
  AntomNotifyURL: string
  AntomRedirectURL: string
}

type AntomSettingsSectionProps = {
  values: AntomSettingsValues
  privateKeyConfigured: boolean
  publicKeyConfigured: boolean
  errors?: Partial<
    Record<'AntomGateway' | 'AntomNotifyURL' | 'AntomRedirectURL', string>
  >
  onValueChange: <K extends keyof AntomSettingsValues>(
    key: K,
    value: AntomSettingsValues[K]
  ) => void
}

export function AntomSettingsSection(props: AntomSettingsSectionProps) {
  const { t } = useTranslation()

  return (
    <div className='space-y-4 pt-4'>
      <div>
        <h3 className='text-lg font-medium'>{t('Antom Checkout')}</h3>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Redirect users to the Antom hosted checkout, where Antom selects the eligible wallets and handles wallet-side currency conversion.'
          )}
        </p>
      </div>

      <Alert>
        <AlertDescription className='text-xs'>
          {t(
            'Configure your Client ID and standard RSA keys from Antom Dashboard. Leave key fields blank unless rotating them.'
          )}
        </AlertDescription>
      </Alert>

      <SettingsSwitchField
        checked={props.values.AntomEnabled}
        onCheckedChange={(value) => props.onValueChange('AntomEnabled', value)}
        label={t('Enable Antom Checkout')}
        className='py-0'
      />

      <div className='grid gap-1.5'>
        <Label htmlFor='antom-display-name'>{t('Wallet entry name')}</Label>
        <Input
          id='antom-display-name'
          value={props.values.AntomDisplayName}
          maxLength={64}
          placeholder={t('Global Wallet Payment')}
          onChange={(event) =>
            props.onValueChange('AntomDisplayName', event.target.value)
          }
        />
        <p className='text-muted-foreground text-xs'>
          {t(
            'Shown to users as the payment method name. Leave blank to use the default name.'
          )}
        </p>
      </div>

      <div className='grid gap-4 md:grid-cols-2'>
        <div className='grid gap-1.5'>
          <Label htmlFor='antom-gateway'>{t('Gateway URL')}</Label>
          <Input
            id='antom-gateway'
            value={props.values.AntomGateway}
            placeholder='https://open-sea-global.alipay.com'
            onChange={(event) =>
              props.onValueChange('AntomGateway', event.target.value)
            }
          />
          {props.errors?.AntomGateway && (
            <p className='text-destructive text-xs'>
              {t(props.errors.AntomGateway)}
            </p>
          )}
        </div>
        <div className='grid gap-1.5'>
          <Label htmlFor='antom-client-id'>{t('Client ID')}</Label>
          <Input
            id='antom-client-id'
            value={props.values.AntomClientId}
            autoComplete='off'
            onChange={(event) =>
              props.onValueChange('AntomClientId', event.target.value)
            }
          />
        </div>
      </div>

      <div className='grid gap-4 md:grid-cols-2'>
        <div className='grid gap-1.5'>
          <Label htmlFor='antom-notify-url'>{t('Notification URL')}</Label>
          <Input
            id='antom-notify-url'
            value={props.values.AntomNotifyURL}
            placeholder='https://example.com/api/user/antom/notify'
            onChange={(event) =>
              props.onValueChange('AntomNotifyURL', event.target.value)
            }
          />
          <p className='text-muted-foreground text-xs'>
            {t('Leave blank to derive it from the configured server address.')}
          </p>
          {props.errors?.AntomNotifyURL && (
            <p className='text-destructive text-xs'>
              {t(props.errors.AntomNotifyURL)}
            </p>
          )}
        </div>
        <div className='grid gap-1.5'>
          <Label htmlFor='antom-redirect-url'>{t('Redirect URL')}</Label>
          <Input
            id='antom-redirect-url'
            value={props.values.AntomRedirectURL}
            placeholder='https://example.com/wallet'
            onChange={(event) =>
              props.onValueChange('AntomRedirectURL', event.target.value)
            }
          />
          <p className='text-muted-foreground text-xs'>
            {t(
              'The payment status and trade number are appended as query parameters.'
            )}
          </p>
          {props.errors?.AntomRedirectURL && (
            <p className='text-destructive text-xs'>
              {t(props.errors.AntomRedirectURL)}
            </p>
          )}
        </div>
      </div>

      <div className='grid gap-4 md:grid-cols-2'>
        <div className='grid gap-1.5'>
          <Label htmlFor='antom-private-key'>{t('Merchant private key')}</Label>
          <Textarea
            id='antom-private-key'
            rows={6}
            value={props.values.AntomMerchantPrivateKey}
            autoComplete='new-password'
            placeholder={t('Leave blank unless rotating the secret')}
            className='font-mono text-xs'
            onChange={(event) =>
              props.onValueChange('AntomMerchantPrivateKey', event.target.value)
            }
          />
          <p className='text-muted-foreground text-xs'>
            {t('Credential status')}:{' '}
            {t(props.privateKeyConfigured ? 'Configured' : 'Not configured')}
          </p>
        </div>
        <div className='grid gap-1.5'>
          <Label htmlFor='antom-public-key'>{t('Antom public key')}</Label>
          <Textarea
            id='antom-public-key'
            rows={6}
            value={props.values.AntomPublicKey}
            autoComplete='new-password'
            placeholder={t('Leave blank unless rotating the secret')}
            className='font-mono text-xs'
            onChange={(event) =>
              props.onValueChange('AntomPublicKey', event.target.value)
            }
          />
          <p className='text-muted-foreground text-xs'>
            {t('Credential status')}:{' '}
            {t(props.publicKeyConfigured ? 'Configured' : 'Not configured')}
          </p>
        </div>
      </div>
    </div>
  )
}

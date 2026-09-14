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
import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect, useMemo } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateBotProtectionSettings } from '../hooks/use-update-option'

const isHTTPURL = (value: string): boolean => {
  try {
    const parsed = new URL(value)
    return (
      (parsed.protocol === 'http:' || parsed.protocol === 'https:') &&
      !parsed.username &&
      !parsed.password
    )
  } catch {
    return false
  }
}

const createBotProtectionSchema = (
  t: (key: string) => string,
  secretConfigured: boolean
) =>
  z
    .object({
      TurnstileCheckEnabled: z.boolean(),
      TurnstileProvider: z.enum(['cloudflare', 'custom']),
      TurnstileSiteKey: z.string(),
      TurnstileSecretKey: z.string(),
      TurnstileWidgetScriptURL: z.string(),
      TurnstileWidgetEndpoint: z.string(),
      TurnstileVerifyURL: z.string(),
      TurnstileAction: z.string().max(128),
      ClearTurnstileSecretKey: z.boolean(),
    })
    .superRefine((values, context) => {
      const enabled = values.TurnstileCheckEnabled
      const secretAvailable =
        !values.ClearTurnstileSecretKey &&
        (values.TurnstileSecretKey.trim() !== '' || secretConfigured)

      if (
        values.ClearTurnstileSecretKey &&
        values.TurnstileSecretKey.trim() !== ''
      ) {
        context.addIssue({
          code: 'custom',
          path: ['TurnstileSecretKey'],
          message: t('Secret cannot be entered and cleared at the same time'),
        })
      }

      if (
        enabled &&
        values.TurnstileProvider === 'cloudflare' &&
        !secretAvailable
      ) {
        context.addIssue({
          code: 'custom',
          path: ['TurnstileSecretKey'],
          message: t('Secret key is required'),
        })
      }
      if (
        enabled &&
        values.TurnstileProvider === 'cloudflare' &&
        values.TurnstileSiteKey.trim() === ''
      ) {
        context.addIssue({
          code: 'custom',
          path: ['TurnstileSiteKey'],
          message: t('Site key is required'),
        })
      }

      const customURLFields = [
        ['TurnstileWidgetScriptURL', values.TurnstileWidgetScriptURL],
        ['TurnstileWidgetEndpoint', values.TurnstileWidgetEndpoint],
        ['TurnstileVerifyURL', values.TurnstileVerifyURL],
      ] as const
      if (values.TurnstileProvider !== 'custom') return
      for (const [field, rawValue] of customURLFields) {
        const value = rawValue.trim()
        if (enabled && value === '') {
          context.addIssue({
            code: 'custom',
            path: [field],
            message: t('URL is required'),
          })
          continue
        }
        if (value !== '' && !isHTTPURL(value)) {
          context.addIssue({
            code: 'custom',
            path: [field],
            message: t('Must be a valid URL'),
          })
        }
      }
    })

type BotProtectionFormValues = z.infer<
  ReturnType<typeof createBotProtectionSchema>
>

type BotProtectionSectionProps = {
  defaultValues: Omit<BotProtectionFormValues, 'ClearTurnstileSecretKey'> & {
    TurnstileSecretKeyConfigured: boolean
  }
}

export function BotProtectionSection({
  defaultValues,
}: BotProtectionSectionProps) {
  const { t } = useTranslation()
  const updateSettings = useUpdateBotProtectionSettings()
  const schema = useMemo(
    () =>
      createBotProtectionSchema(t, defaultValues.TurnstileSecretKeyConfigured),
    [defaultValues.TurnstileSecretKeyConfigured, t]
  )
  const formDefaults = useMemo<BotProtectionFormValues>(
    () => ({
      TurnstileCheckEnabled: defaultValues.TurnstileCheckEnabled,
      TurnstileProvider: defaultValues.TurnstileProvider,
      TurnstileSiteKey: defaultValues.TurnstileSiteKey,
      TurnstileSecretKey: '',
      TurnstileWidgetScriptURL: defaultValues.TurnstileWidgetScriptURL,
      TurnstileWidgetEndpoint: defaultValues.TurnstileWidgetEndpoint,
      TurnstileVerifyURL: defaultValues.TurnstileVerifyURL,
      TurnstileAction: defaultValues.TurnstileAction,
      ClearTurnstileSecretKey: false,
    }),
    [defaultValues]
  )

  const form = useForm<BotProtectionFormValues>({
    resolver: zodResolver(schema),
    defaultValues: formDefaults,
  })

  useEffect(() => {
    form.reset(formDefaults)
  }, [form, formDefaults])

  const onSubmit = async (data: BotProtectionFormValues) => {
    await updateSettings.mutateAsync({
      enabled: data.TurnstileCheckEnabled,
      provider: data.TurnstileProvider,
      site_key: data.TurnstileSiteKey.trim(),
      secret_key: data.TurnstileSecretKey.trim(),
      widget_script_url: data.TurnstileWidgetScriptURL.trim(),
      widget_endpoint: data.TurnstileWidgetEndpoint.trim(),
      verify_url: data.TurnstileVerifyURL.trim(),
      action: data.TurnstileAction.trim(),
      clear_secret: data.ClearTurnstileSecretKey,
    })
  }

  const provider = form.watch('TurnstileProvider')
  const clearSecret = form.watch('ClearTurnstileSecretKey')
  let secretPlaceholder = t('Enter the verification secret key')
  if (defaultValues.TurnstileSecretKeyConfigured) {
    secretPlaceholder = t('Configured; leave blank to keep the current secret')
  } else if (provider === 'custom') {
    secretPlaceholder = t('Optional verification secret key')
  }

  return (
    <SettingsSection title={t('Bot Protection')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)} autoComplete='off'>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateSettings.isPending}
          />
          <FormField
            control={form.control}
            name='TurnstileCheckEnabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable human verification')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Protect login, registration, password reset, and check-in with human verification'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          <FormField
            control={form.control}
            name='TurnstileProvider'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Verification provider')}</FormLabel>
                <Select value={field.value} onValueChange={field.onChange}>
                  <FormControl>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent>
                    <SelectItem value='cloudflare'>
                      Cloudflare Turnstile
                    </SelectItem>
                    <SelectItem value='custom'>
                      {t('Custom slider (Captcha88 compatible)')}
                    </SelectItem>
                  </SelectContent>
                </Select>
                <FormDescription>
                  {t(
                    'Provider settings are stored in the database and can be switched without changing environment variables'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          {provider === 'cloudflare' && (
            <FormField
              control={form.control}
              name='TurnstileSiteKey'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Site Key')}</FormLabel>
                  <FormControl>
                    <Input
                      placeholder={t('Your Turnstile site key')}
                      autoComplete='off'
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          )}

          {provider === 'custom' && (
            <>
              <FormField
                control={form.control}
                name='TurnstileWidgetScriptURL'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Widget script URL')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder='https://captcha.example.com/widget.js'
                        autoComplete='off'
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Browser URL used to load the slider widget script')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='TurnstileWidgetEndpoint'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Widget API endpoint')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder='https://captcha.example.com'
                        autoComplete='off'
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Endpoint passed to the custom slider widget')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='TurnstileVerifyURL'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Server verification URL')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder='https://captcha.example.com/turnstile/v0/siteverify'
                        autoComplete='off'
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'NewAPI sends the token and secret to this URL from the server'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='TurnstileSiteKey'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Site Key (optional)')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t(
                          'Optional site key for the custom widget'
                        )}
                        autoComplete='off'
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='TurnstileAction'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Verification action')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder='register'
                        autoComplete='off'
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Action name passed to the custom slider widget')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </>
          )}

          <FormField
            control={form.control}
            name='TurnstileSecretKey'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {provider === 'custom'
                    ? t('Secret Key (optional)')
                    : t('Secret Key')}
                </FormLabel>
                <FormControl>
                  <Input
                    type='password'
                    placeholder={secretPlaceholder}
                    autoComplete='new-password'
                    disabled={clearSecret}
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {provider === 'custom'
                    ? t(
                        'Leave blank when the custom verification service does not require a secret'
                      )
                    : t(
                        'The secret is only sent to the server and is never returned to the browser'
                      )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          {defaultValues.TurnstileSecretKeyConfigured && (
            <FormField
              control={form.control}
              name='ClearTurnstileSecretKey'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Clear stored secret')}</FormLabel>
                    <FormDescription>
                      {provider === 'custom'
                        ? t(
                            'Clear the stored secret when the custom service does not require one'
                          )
                        : t(
                            'Disable human verification before clearing the stored secret'
                          )}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={(checked) => {
                        field.onChange(checked)
                        if (checked) {
                          form.setValue('TurnstileSecretKey', '')
                        }
                      }}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />
          )}
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}

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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CheckCircle2, TestTube2 } from 'lucide-react'
import { forwardRef, useEffect, useImperativeHandle, type Ref } from 'react'
import { useForm, type Resolver } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
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
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import {
  fetchAffiliateAlipayPayoutSettings,
  testAffiliateAlipayPayoutSettings,
  updateAffiliateAlipayPayoutSettings,
} from '@/features/referral/api'
import { getUserFacingErrorMessage } from '@/lib/user-facing-error'

import {
  SettingsForm,
  SettingsFormGrid,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsSection } from '../components/settings-section'

const schema = z.object({
  enabled: z.boolean(),
  appId: z
    .string()
    .trim()
    .regex(/^$|^\d{16}$/),
  transferTitle: z.string().trim().min(1).max(64),
  privateKey: z.string(),
  appCertificate: z.string(),
  alipayPublicCertificate: z.string(),
  alipayRootCertificate: z.string(),
})

type AffiliatePayoutSettingsValues = z.infer<typeof schema>

export type AffiliatePayoutSettingsHandle = {
  save: () => Promise<boolean>
}

export type AffiliatePayoutSettingsFormState = {
  isDirty: boolean
  isSaving: boolean
}

type AffiliatePayoutSettingsSectionProps = {
  onFormStateChange?: (state: AffiliatePayoutSettingsFormState) => void
}

export const AffiliatePayoutSettingsSection = forwardRef(
  function AffiliatePayoutSettingsSection(
    props: AffiliatePayoutSettingsSectionProps,
    ref: Ref<AffiliatePayoutSettingsHandle>
  ) {
    const { onFormStateChange } = props
    const { t } = useTranslation()
    const queryClient = useQueryClient()
    const settingsQuery = useQuery({
      queryKey: ['affiliate-alipay-payout-settings'],
      queryFn: fetchAffiliateAlipayPayoutSettings,
    })
    const form = useForm<AffiliatePayoutSettingsValues>({
      resolver: zodResolver(schema) as Resolver<AffiliatePayoutSettingsValues>,
      defaultValues: {
        enabled: false,
        appId: '',
        transferTitle: '88API 推广佣金结算',
        privateKey: '',
        appCertificate: '',
        alipayPublicCertificate: '',
        alipayRootCertificate: '',
      },
    })
    useEffect(() => {
      if (!settingsQuery.data) return
      form.reset({
        enabled: settingsQuery.data.enabled,
        appId: settingsQuery.data.app_id,
        transferTitle: settingsQuery.data.transfer_title,
        privateKey: '',
        appCertificate: '',
        alipayPublicCertificate: '',
        alipayRootCertificate: '',
      })
    }, [form, settingsQuery.data])

    const updateMutation = useMutation({
      mutationFn: updateAffiliateAlipayPayoutSettings,
      onSuccess: async () => {
        toast.success(t('Alipay direct payout settings saved'))
        await queryClient.invalidateQueries({
          queryKey: ['affiliate-alipay-payout-settings'],
        })
        await queryClient.invalidateQueries({
          queryKey: ['admin-affiliate-payout-provider'],
        })
      },
      onError: (error) => toast.error(getUserFacingErrorMessage(error)),
    })
    const testMutation = useMutation({
      mutationFn: testAffiliateAlipayPayoutSettings,
      onSuccess: () =>
        toast.success(
          t('Alipay signature and transfer-query credentials are valid')
        ),
      onError: (error) => toast.error(getUserFacingErrorMessage(error)),
    })
    const saving = updateMutation.isPending || form.formState.isSubmitting
    const onSubmit = async (values: AffiliatePayoutSettingsValues) => {
      await updateMutation.mutateAsync({
        enabled: values.enabled,
        app_id: values.appId,
        transfer_title: values.transferTitle,
        private_key: values.privateKey,
        app_certificate: values.appCertificate,
        alipay_public_certificate: values.alipayPublicCertificate,
        alipay_root_certificate: values.alipayRootCertificate,
      })
      form.reset({
        ...values,
        privateKey: '',
        appCertificate: '',
        alipayPublicCertificate: '',
        alipayRootCertificate: '',
      })
    }

    useImperativeHandle(ref, () => ({
      save: async () => {
        if (!form.formState.isDirty) return true

        let saved = false
        await form.handleSubmit(
          async (values) => {
            await onSubmit(values)
            saved = true
          },
          () => {
            saved = false
          }
        )()
        return saved
      },
    }))

    useEffect(() => {
      onFormStateChange?.({
        isDirty: form.formState.isDirty,
        isSaving: saving,
      })
    }, [form.formState.isDirty, onFormStateChange, saving])

    return (
      <SettingsSection title={t('Alipay direct payout')}>
        <Form {...form}>
          <SettingsForm
            onSubmit={form.handleSubmit(onSubmit)}
            autoComplete='off'
          >
            <SettingsSwitchItem>
              <SettingsSwitchContent>
                <FormLabel>{t('Enable Alipay direct payout')}</FormLabel>
                <FormDescription>
                  {t(
                    'Approved commission payouts can be paid directly to the submitted Alipay name and account. Manual confirmation remains available.'
                  )}
                </FormDescription>
              </SettingsSwitchContent>
              <FormField
                control={form.control}
                name='enabled'
                render={({ field }) => (
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                      disabled={saving || settingsQuery.isLoading}
                    />
                  </FormControl>
                )}
              />
            </SettingsSwitchItem>

            <div className='flex flex-wrap items-center gap-2 text-sm'>
              <span className='text-muted-foreground'>
                {t('Credential status')}:
              </span>
              <Badge
                variant='outline'
                className={
                  settingsQuery.data?.configured
                    ? 'border-emerald-500/40 bg-emerald-500/10 text-emerald-600'
                    : 'border-amber-500/40 bg-amber-500/10 text-amber-600'
                }
              >
                {settingsQuery.data?.configured
                  ? t('Configured')
                  : t('Not configured')}
              </Badge>
              {settingsQuery.data?.configured ? (
                <span className='text-muted-foreground inline-flex items-center gap-1 text-xs'>
                  <CheckCircle2 className='size-3.5' />
                  {t('Official certificate mode')}
                </span>
              ) : null}
            </div>

            <SettingsFormGrid>
              <FormField
                control={form.control}
                name='appId'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Alipay App ID')}</FormLabel>
                    <FormControl>
                      <Input {...field} disabled={saving} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='transferTitle'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {t('Payout note (shown in Alipay transfer records)')}
                    </FormLabel>
                    <FormControl>
                      <Input {...field} disabled={saving} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </SettingsFormGrid>

            <div
              data-settings-form-span='full'
              className='grid min-w-0 gap-x-5 gap-y-6 lg:grid-cols-2'
            >
              <FormField
                control={form.control}
                name='privateKey'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Application private key')}</FormLabel>
                    <FormControl>
                      <Textarea
                        {...field}
                        rows={4}
                        className='font-mono text-xs'
                        placeholder={
                          settingsQuery.data?.private_key_configured
                            ? t('Saved; leave blank to keep unchanged')
                            : t('Paste the RSA2 application private key')
                        }
                        disabled={saving}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='appCertificate'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Application public certificate')}</FormLabel>
                    <FormControl>
                      <Textarea
                        {...field}
                        rows={4}
                        className='font-mono text-xs'
                        placeholder={
                          settingsQuery.data?.app_certificate_configured
                            ? t('Saved; leave blank to keep unchanged')
                            : t(
                                'Paste the application public certificate (.crt)'
                              )
                        }
                        disabled={saving}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='alipayPublicCertificate'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Alipay public certificate')}</FormLabel>
                    <FormControl>
                      <Textarea
                        {...field}
                        rows={4}
                        className='font-mono text-xs'
                        placeholder={
                          settingsQuery.data
                            ?.alipay_public_certificate_configured
                            ? t('Saved; leave blank to keep unchanged')
                            : t('Paste the Alipay public certificate (.crt)')
                        }
                        disabled={saving}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='alipayRootCertificate'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Alipay root certificate')}</FormLabel>
                    <FormControl>
                      <Textarea
                        {...field}
                        rows={4}
                        className='font-mono text-xs'
                        placeholder={
                          settingsQuery.data?.alipay_root_certificate_configured
                            ? t('Saved; leave blank to keep unchanged')
                            : t(
                                'Paste the complete Alipay root certificate bundle (.crt)'
                              )
                        }
                        disabled={saving}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            <div className='flex flex-wrap justify-end gap-2'>
              <Button
                type='button'
                variant='outline'
                disabled={
                  saving ||
                  testMutation.isPending ||
                  !settingsQuery.data?.configured
                }
                onClick={() => testMutation.mutate()}
              >
                <TestTube2
                  className={
                    testMutation.isPending ? 'size-4 animate-pulse' : 'size-4'
                  }
                />
                {t('Test credentials')}
              </Button>
            </div>
          </SettingsForm>
        </Form>
      </SettingsSection>
    )
  }
)

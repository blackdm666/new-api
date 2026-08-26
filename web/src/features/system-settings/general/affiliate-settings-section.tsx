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
import { useMutation } from '@tanstack/react-query'
import { useRef, useState } from 'react'
import { useForm, type Resolver } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { Alert, AlertDescription } from '@/components/ui/alert'
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
import { updateAffiliateSettings } from '@/features/referral/api'
import { getUserFacingErrorMessage } from '@/lib/user-facing-error'

import { FormNavigationGuard } from '../components/form-navigation-guard'
import {
  SettingsControlGroup,
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import {
  AffiliatePayoutSettingsSection,
  type AffiliatePayoutSettingsFormState,
  type AffiliatePayoutSettingsHandle,
} from './affiliate-payout-settings-section'

const schema = z.object({
  enabled: z.boolean(),
  autoApprove: z.boolean(),
  juniorRate: z.coerce.number().min(0).max(100),
  advancedRate: z.coerce.number().min(0).max(100),
  goldRate: z.coerce.number().min(0).max(100),
  upgradeThreshold: z.coerce.number().int().min(1).max(1000000),
  goldUpgradeThreshold: z.coerce.number().int().min(1).max(1000000),
  upgradeAmountThreshold: z.coerce.number().min(0.01).max(1000000000000),
  goldUpgradeAmountThreshold: z.coerce.number().min(0.01).max(1000000000000),
})

type AffiliateSettingsValues = z.infer<typeof schema>

export function AffiliateSettingsSection(props: {
  defaultValues: AffiliateSettingsValues
  fixedInviterReward: number
}) {
  const { t } = useTranslation()
  const payoutSettingsRef = useRef<AffiliatePayoutSettingsHandle>(null)
  const [payoutFormState, setPayoutFormState] =
    useState<AffiliatePayoutSettingsFormState>({
      isDirty: false,
      isSaving: false,
    })
  const form = useForm<AffiliateSettingsValues>({
    resolver: zodResolver(schema) as Resolver<AffiliateSettingsValues>,
    defaultValues: props.defaultValues,
  })
  const updateMutation = useMutation({
    mutationFn: updateAffiliateSettings,
    onSuccess: () => toast.success(t('Referral commission settings saved')),
    onError: (error) => toast.error(getUserFacingErrorMessage(error)),
  })
  const enabled = form.watch('enabled')

  const onSubmit = async (values: AffiliateSettingsValues) => {
    if (values.goldUpgradeThreshold <= values.upgradeThreshold) {
      form.setError('goldUpgradeThreshold', {
        message: t(
          'Gold upgrade threshold must be greater than advanced upgrade threshold.'
        ),
      })
      return false
    }
    if (values.goldUpgradeAmountThreshold <= values.upgradeAmountThreshold) {
      form.setError('goldUpgradeAmountThreshold', {
        message: t(
          'Gold upgrade top-up amount must be greater than advanced upgrade top-up amount.'
        ),
      })
      return false
    }
    await updateMutation.mutateAsync({
      enabled: values.enabled,
      auto_approve: values.autoApprove,
      default_rate_basis_points: Math.round(values.juniorRate * 100),
      group_rates: {
        default: Math.round(values.juniorRate * 100),
        高级推广: Math.round(values.advancedRate * 100),
        金牌推广: Math.round(values.goldRate * 100),
      },
      upgrade_invitees_threshold: values.upgradeThreshold,
      gold_upgrade_invitees_threshold: values.goldUpgradeThreshold,
      upgrade_top_up_amount_threshold_cents: Math.round(
        values.upgradeAmountThreshold * 100
      ),
      gold_upgrade_top_up_amount_threshold_cents: Math.round(
        values.goldUpgradeAmountThreshold * 100
      ),
    })
    form.reset(values)
    return true
  }

  const saveAllSettings = async () => {
    let mainSettingsSaved = true
    if (form.formState.isDirty) {
      mainSettingsSaved = false
      await form.handleSubmit(
        async (values) => {
          mainSettingsSaved = await onSubmit(values)
        },
        () => {
          mainSettingsSaved = false
        }
      )()
    }
    if (mainSettingsSaved && payoutFormState.isDirty) {
      await payoutSettingsRef.current?.save()
    }
  }

  const isSaving =
    updateMutation.isPending ||
    form.formState.isSubmitting ||
    payoutFormState.isSaving
  const isDirty = form.formState.isDirty || payoutFormState.isDirty

  return (
    <>
      <FormNavigationGuard when={isDirty} />
      <SettingsSection title={t('Referral Commission')}>
        <Form {...form}>
          <SettingsForm
            onSubmit={(event) => {
              event.preventDefault()
              void saveAllSettings()
            }}
            autoComplete='off'
          >
            <SettingsPageFormActions
              onSave={() => void saveAllSettings()}
              isSaving={isSaving}
              isSaveDisabled={!isDirty}
              saveLabel='Save referral commission settings'
            />

            {props.fixedInviterReward > 0 ? (
              <Alert>
                <AlertDescription>
                  {t(
                    'The legacy fixed inviter reward is still enabled. Set Inviter Reward to 0 if you only want percentage-based top-up commission.'
                  )}
                </AlertDescription>
              </Alert>
            ) : null}

            <FormField
              control={form.control}
              name='enabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Enable top-up commission')}</FormLabel>
                    <FormDescription>
                      {t(
                        'Create one commission ledger entry when an invited user completes an eligible Epay or Antom wallet top-up. Stripe, subscriptions, balance transfers, redemption codes, and manual balance changes are excluded.'
                      )}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                      disabled={updateMutation.isPending}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />

            <FormField
              control={form.control}
              name='autoApprove'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>
                      {t('Automatically approve commissions')}
                    </FormLabel>
                    <FormDescription>
                      {t(
                        'When enabled, valid wallet top-up commissions are credited immediately and still appear in the commission ledger. When disabled, new commissions wait for manual review.'
                      )}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                      disabled={!enabled || updateMutation.isPending}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />

            <div data-settings-form-span='full' className='grid min-w-0 gap-4'>
              <SettingsControlGroup className='space-y-4 p-4'>
                <h3 className='text-sm font-semibold'>
                  {t('Junior promoter')}
                </h3>
                <div className='grid min-w-0 gap-4 md:grid-cols-2 xl:grid-cols-3'>
                  <RateField
                    control={form.control}
                    name='juniorRate'
                    label={t('Junior promoter rate')}
                    disabled={!enabled || updateMutation.isPending}
                  />
                </div>
              </SettingsControlGroup>

              <SettingsControlGroup className='space-y-4 p-4'>
                <h3 className='text-sm font-semibold'>
                  {t('Advanced promoter')}
                </h3>
                <div className='grid min-w-0 gap-4 md:grid-cols-2 xl:grid-cols-3'>
                  <RateField
                    control={form.control}
                    name='advancedRate'
                    label={t('Advanced promoter rate')}
                    disabled={!enabled || updateMutation.isPending}
                  />
                  <InviteeThresholdField
                    control={form.control}
                    name='upgradeThreshold'
                    label={t('Advanced promoter invitee threshold')}
                    disabled={!enabled || updateMutation.isPending}
                  />
                  <MoneyThresholdField
                    control={form.control}
                    name='upgradeAmountThreshold'
                    label={t('Advanced promoter top-up amount threshold')}
                    disabled={!enabled || updateMutation.isPending}
                  />
                </div>
              </SettingsControlGroup>

              <SettingsControlGroup className='space-y-4 p-4'>
                <h3 className='text-sm font-semibold'>{t('Gold promoter')}</h3>
                <div className='grid min-w-0 gap-4 md:grid-cols-2 xl:grid-cols-3'>
                  <RateField
                    control={form.control}
                    name='goldRate'
                    label={t('Gold promoter rate')}
                    disabled={!enabled || updateMutation.isPending}
                  />
                  <InviteeThresholdField
                    control={form.control}
                    name='goldUpgradeThreshold'
                    label={t('Gold promoter invitee threshold')}
                    disabled={!enabled || updateMutation.isPending}
                  />
                  <MoneyThresholdField
                    control={form.control}
                    name='goldUpgradeAmountThreshold'
                    label={t('Gold promoter top-up amount threshold')}
                    disabled={!enabled || updateMutation.isPending}
                  />
                </div>
              </SettingsControlGroup>
            </div>
          </SettingsForm>
        </Form>
      </SettingsSection>
      <AffiliatePayoutSettingsSection
        ref={payoutSettingsRef}
        onFormStateChange={setPayoutFormState}
      />
    </>
  )
}

function MoneyThresholdField(props: {
  control: ReturnType<typeof useForm<AffiliateSettingsValues>>['control']
  name: 'upgradeAmountThreshold' | 'goldUpgradeAmountThreshold'
  label: string
  disabled: boolean
}) {
  const { t } = useTranslation()
  return (
    <FormField
      control={props.control}
      name={props.name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{props.label}</FormLabel>
          <FormControl>
            <div className='relative'>
              <Input
                type='number'
                min={0.01}
                max={1000000000000}
                step={0.01}
                className='pr-9'
                disabled={props.disabled}
                {...field}
              />
              <span className='text-muted-foreground pointer-events-none absolute top-1/2 right-3 -translate-y-1/2 text-sm'>
                {t('CNY')}
              </span>
            </div>
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}

function RateField(props: {
  control: ReturnType<typeof useForm<AffiliateSettingsValues>>['control']
  name: 'juniorRate' | 'advancedRate' | 'goldRate'
  label: string
  disabled: boolean
}) {
  const { t } = useTranslation()
  return (
    <FormField
      control={props.control}
      name={props.name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{props.label}</FormLabel>
          <FormControl>
            <div className='relative'>
              <Input
                type='number'
                min={0}
                max={100}
                step={0.01}
                className='pr-9'
                disabled={props.disabled}
                {...field}
              />
              <span className='text-muted-foreground pointer-events-none absolute top-1/2 right-3 -translate-y-1/2 text-sm'>
                %
              </span>
            </div>
          </FormControl>
          <FormMessage>
            {field.value > 100 ? t('Maximum is 100%') : null}
          </FormMessage>
        </FormItem>
      )}
    />
  )
}

function InviteeThresholdField(props: {
  control: ReturnType<typeof useForm<AffiliateSettingsValues>>['control']
  name: 'upgradeThreshold' | 'goldUpgradeThreshold'
  label: string
  disabled: boolean
}) {
  return (
    <FormField
      control={props.control}
      name={props.name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{props.label}</FormLabel>
          <FormControl>
            <Input
              type='number'
              min={1}
              max={1000000}
              disabled={props.disabled}
              {...field}
            />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}

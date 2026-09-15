/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, test, vi } from 'vitest'

const i18n = (await import('i18next')).default
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { BalanceQueryEditorDialog } =
  await import('../balance-query-editor-dialog')

await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

describe('balance query editor dialog', () => {
  test('previews the native path against a version-free Base URL', () => {
    render(
      <I18nextProvider i18n={i18n}>
        <BalanceQueryEditorDialog
          open
          onOpenChange={() => undefined}
          value={{
            mode: 'new_api',
            auth: {
              type: 'header',
              name: 'Authorization',
              value: 'account-access-token',
            },
          }}
          channelType={60}
          baseURL='https://upstream.example'
          onSave={() => undefined}
        />
      </I18nextProvider>
    )

    expect(
      screen.getByText('https://upstream.example/api/user/self')
    ).toBeVisible()
    expect(screen.getByLabelText('Upstream account user ID')).toBeVisible()
    expect(
      screen.queryByText('https://upstream.example/v1/api/user/self')
    ).not.toBeInTheDocument()
  })

  test('does not allow NewAPI account balance without an account token', () => {
    render(
      <I18nextProvider i18n={i18n}>
        <BalanceQueryEditorDialog
          open
          onOpenChange={() => undefined}
          value={{ mode: 'new_api' }}
          channelType={60}
          baseURL='https://upstream.example'
          onSave={() => undefined}
        />
      </I18nextProvider>
    )

    expect(screen.getByLabelText('Account access token')).toBeVisible()
    expect(screen.getByLabelText('Upstream account user ID')).toBeVisible()
    expect(
      screen.getByRole('button', { name: 'Save balance query' })
    ).toBeDisabled()
  })

  test('shows the account token fields while following a NewAPI channel type', () => {
    render(
      <I18nextProvider i18n={i18n}>
        <BalanceQueryEditorDialog
          open
          onOpenChange={() => undefined}
          value={{
            mode: 'auto',
            auth: {
              type: 'header',
              name: 'Authorization',
              value: '',
            },
            auth_configured: true,
            auth_masked: 'abcd••••wxyz',
            account_user_id: '300',
          }}
          channelType={60}
          baseURL='https://upstream.example'
          onSave={() => undefined}
        />
      </I18nextProvider>
    )

    expect(screen.getByLabelText('Account access token')).toBeVisible()
    expect(screen.getByLabelText('Upstream account user ID')).toHaveValue('300')
    expect(
      screen.getByRole('button', { name: 'Save balance query' })
    ).toBeEnabled()
  })

  test('keeps the replacement field empty without a duplicate masked-token label', () => {
    render(
      <I18nextProvider i18n={i18n}>
        <BalanceQueryEditorDialog
          open
          onOpenChange={() => undefined}
          value={{
            mode: 'new_api',
            auth: {
              type: 'header',
              name: 'Authorization',
              value: '',
            },
            auth_configured: true,
            auth_masked: 'rE9P••••••••F4g=',
          }}
          channelType={60}
          baseURL='https://upstream.example'
          onSave={() => undefined}
        />
      </I18nextProvider>
    )

    expect(
      screen.queryByText('Current token: rE9P••••••••F4g=')
    ).not.toBeInTheDocument()
    expect(screen.getByLabelText('Account access token')).toHaveValue('')
    expect(
      screen.getByRole('button', { name: 'Show full token' })
    ).toBeVisible()
    expect(screen.getByRole('button', { name: 'Copy token' })).toBeVisible()
    expect(
      screen.getByRole('button', { name: 'Save balance query' })
    ).toBeEnabled()
  })

  test('reveals and hides the complete saved token in the account token field', () => {
    render(
      <I18nextProvider i18n={i18n}>
        <BalanceQueryEditorDialog
          open
          onOpenChange={() => undefined}
          value={{
            mode: 'new_api',
            auth: { type: 'header', name: 'Authorization', value: '' },
            auth_configured: true,
            auth_masked: 'acco••••oken',
          }}
          channelType={60}
          baseURL='https://upstream.example'
          savedToken='account-access-token'
          canRevealSavedToken
          onSave={() => undefined}
        />
      </I18nextProvider>
    )

    const input = screen.getByLabelText('Account access token')
    expect(input).toHaveAttribute('type', 'password')
    expect(input).toHaveValue('account-access-token')

    fireEvent.click(screen.getByRole('button', { name: 'Show full token' }))
    expect(input).toHaveAttribute('type', 'text')

    fireEvent.click(screen.getByRole('button', { name: 'Hide token' }))
    expect(input).toHaveAttribute('type', 'password')
  })

  test('requests a secure copy when the saved token is still masked', async () => {
    const onCopySavedToken = vi.fn().mockResolvedValue(undefined)
    render(
      <I18nextProvider i18n={i18n}>
        <BalanceQueryEditorDialog
          open
          onOpenChange={() => undefined}
          value={{
            mode: 'new_api',
            auth: { type: 'header', name: 'Authorization', value: '' },
            auth_configured: true,
            auth_masked: 'acco••••oken',
          }}
          channelType={60}
          baseURL='https://upstream.example'
          canRevealSavedToken
          onCopySavedToken={onCopySavedToken}
          onSave={() => undefined}
        />
      </I18nextProvider>
    )

    fireEvent.click(screen.getByRole('button', { name: 'Copy token' }))
    await waitFor(() => expect(onCopySavedToken).toHaveBeenCalledOnce())
  })

  test('keeps an incomplete custom mapping unsavable', () => {
    render(
      <I18nextProvider i18n={i18n}>
        <BalanceQueryEditorDialog
          open
          onOpenChange={() => undefined}
          value={{ mode: 'custom' }}
          channelType={8}
          baseURL='https://custom.example'
          onSave={() => undefined}
        />
      </I18nextProvider>
    )

    expect(
      screen.getByRole('button', { name: 'Save balance query' })
    ).toBeDisabled()
    expect(screen.getByText('Configuration incomplete')).toBeVisible()
  })

  test('shows method, calculation, headers, and pre-save test controls for custom mapping', () => {
    render(
      <I18nextProvider i18n={i18n}>
        <BalanceQueryEditorDialog
          open
          onOpenChange={() => undefined}
          value={{
            mode: 'custom',
            url: 'https://api.openmodel.ai/web/v1/self',
            method: 'GET',
            auth: {
              type: 'header',
              name: 'Authorization',
              value: 'Bearer account-token',
            },
            remaining_mode: 'total_minus_used',
            response: {
              total_path: 'balance',
              used_path: 'frozen_balance',
            },
            multiplier: '0.000001',
          }}
          channelType={8}
          baseURL='https://custom.example'
          channelId={74}
          onSave={() => undefined}
        />
      </I18nextProvider>
    )

    expect(screen.getByLabelText('Request method')).toBeVisible()
    expect(screen.getByLabelText('Remaining balance calculation')).toBeVisible()
    expect(screen.getByLabelText('Total path *')).toHaveValue('balance')
    expect(screen.getByLabelText('Used or frozen path *')).toHaveValue(
      'frozen_balance'
    )
    expect(screen.getByRole('button', { name: 'Add header' })).toBeVisible()
    expect(screen.getByRole('button', { name: 'Test query' })).toBeEnabled()

    fireEvent.click(
      screen.getByRole('switch', { name: 'Automatic balance refresh' })
    )
    expect(screen.getByLabelText('Refresh interval in minutes')).toHaveValue(15)
    fireEvent.click(
      screen.getByRole('switch', { name: 'Low balance administrator alert' })
    )
    expect(screen.getByLabelText('Low balance threshold')).toBeVisible()
  })

  test('shows the Vertex trial credit activation steps and derived table', () => {
    render(
      <I18nextProvider i18n={i18n}>
        <BalanceQueryEditorDialog
          open
          onOpenChange={() => undefined}
          value={{
            mode: 'gcp_trial_credit',
            gcp_trial: {
              billing_account_id: '0112D2-3D1562-101A70',
              query_project_id: 'api-505117',
              dataset_id: 'billing_export',
              credential_channel_id: 69,
              total_amount: '300',
              baseline_used: '132',
              baseline_at: 1_787_580_000,
            },
          }}
          channelType={41}
          baseURL=''
          onSave={() => undefined}
        />
      </I18nextProvider>
    )

    expect(
      screen.getByText('Vertex trial credit activation steps')
    ).toBeVisible()
    expect(
      screen.getByText(
        'api-505117.billing_export.gcp_billing_export_v1_0112D2_3D1562_101A70'
      )
    ).toBeVisible()
    expect(screen.getByLabelText('Trial credit total')).toHaveValue('300')
    expect(screen.getByLabelText('Historical used baseline')).toHaveValue('132')
    expect(
      screen.getByRole('button', { name: 'Save balance query' })
    ).toBeEnabled()
  })
})

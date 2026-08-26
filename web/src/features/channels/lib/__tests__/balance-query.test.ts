/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { describe, expect, test } from 'vitest'

import {
  BALANCE_QUERY_MODE_OPTIONS,
  buildBalanceQueryPreviewURL,
  createBalanceQueryConfig,
  getEffectiveBalanceQueryPath,
  getGCPTrialBillingTable,
  parseBalanceQueryConfig,
  stringifyBalanceQueryConfig,
  validateBalanceQueryConfig,
} from '../balance-query'
import { formatChannelBalanceInfo } from '../channel-utils'

describe('channel balance query configuration', () => {
  test('keeps Base URL version-free and appends the complete query path', () => {
    const path = getEffectiveBalanceQueryPath({ mode: 'new_api' }, 60)

    expect(path).toBe('/api/user/self')
    expect(buildBalanceQueryPreviewURL('https://upstream.example/', path)).toBe(
      'https://upstream.example/api/user/self'
    )
  })

  test('uses native NewAPI and Sub2API paths when following channel type', () => {
    expect(getEffectiveBalanceQueryPath({ mode: 'auto' }, 60)).toBe(
      '/api/user/self'
    )
    expect(getEffectiveBalanceQueryPath({ mode: 'auto' }, 59)).toBe('/v1/usage')
    expect(getEffectiveBalanceQueryPath({ mode: 'auto' }, 1)).toBeNull()
  })

  test('requires a dedicated account token for NewAPI account balance', () => {
    const config = createBalanceQueryConfig('new_api')
    expect(config.auth?.value).toBe('')
    expect(validateBalanceQueryConfig(config)).toContain(
      'Account access token is required'
    )

    if (!config.auth) throw new Error('expected account auth config')
    config.auth.value = 'account-access-token'
    expect(validateBalanceQueryConfig(config)).toBeNull()

    config.account_user_id = 'invalid'
    expect(validateBalanceQueryConfig(config)).toContain('positive integer')

    config.account_user_id = '300'
    expect(validateBalanceQueryConfig(config)).toBeNull()
    expect(stringifyBalanceQueryConfig(config)).toContain(
      'account-access-token'
    )
    expect(stringifyBalanceQueryConfig(config)).toContain(
      '"account_user_id": "300"'
    )
  })

  test('preserves account credentials while following a NewAPI channel type', () => {
    const config = {
      mode: 'auto' as const,
      auth: {
        type: 'header' as const,
        name: 'Authorization',
        value: '',
      },
      auth_configured: true,
      auth_masked: 'abcd••••wxyz',
      account_user_id: '300',
    }
    const serialized = stringifyBalanceQueryConfig(config)
    expect(serialized).toContain('"mode": "auto"')
    expect(serialized).toContain('"auth_configured": true')
    expect(parseBalanceQueryConfig(serialized).account_user_id).toBe('300')
  })

  test('accepts a server-redacted account token without exposing its value', () => {
    const config = {
      mode: 'new_api' as const,
      auth: {
        type: 'header' as const,
        name: 'Authorization',
        value: '',
      },
      auth_configured: true,
      auth_masked: 'rE9P••••••••F4g=',
      account_user_id: '300',
    }

    expect(validateBalanceQueryConfig(config)).toBeNull()
    const serialized = stringifyBalanceQueryConfig(config)
    expect(serialized).toContain('"auth_configured": true')
    expect(serialized).toContain('rE9P••••••••F4g=')
    expect(serialized).not.toContain('account-token-secret')
  })

  test('keeps special upstreams in custom mapping instead of provider presets', () => {
    expect(
      BALANCE_QUERY_MODE_OPTIONS.some((option) =>
        option.label.toLowerCase().includes('hao')
      )
    ).toBe(false)

    const custom = createBalanceQueryConfig('custom')
    expect(custom.path).toBe('')
    expect(custom.response?.remaining_path).toBe('')
    expect(validateBalanceQueryConfig(custom)).toBe(
      'Balance query URL is required'
    )
  })

  test('builds and validates the Vertex trial credit template', () => {
    const config = createBalanceQueryConfig('gcp_trial_credit')
    expect(config.gcp_trial?.total_amount).toBe('300')
    expect(config.gcp_trial?.baseline_used).toBe('132')
    expect(validateBalanceQueryConfig(config)).toBe(
      'Google Cloud billing account ID is required'
    )

    config.gcp_trial = {
      ...config.gcp_trial,
      billing_account_id: '0112D2-3D1562-101A70',
      query_project_id: 'api-505117',
      dataset_id: 'billing_export',
      credential_channel_id: 69,
      total_amount: '300',
      baseline_used: '132',
      baseline_at: 1_787_580_000,
    }
    expect(validateBalanceQueryConfig(config)).toBeNull()
    expect(getGCPTrialBillingTable(config)).toBe(
      'api-505117.billing_export.gcp_billing_export_v1_0112D2_3D1562_101A70'
    )
  })

  test('validates a user-defined cc-switch style mapping and removes auto config', () => {
    const custom = createBalanceQueryConfig('custom')
    custom.url = 'https://api.example.com/v1/user/balance'
    custom.auth = {
      type: 'header',
      name: 'Authorization',
      value: 'Bearer {api_key}',
    }
    custom.response = {
      remaining_path: 'balance',
      total_path: 'total',
      used_path: 'used',
      currency_path: 'currency',
      active_path: 'is_active',
    }

    expect(validateBalanceQueryConfig(custom)).toBeNull()
    expect(stringifyBalanceQueryConfig(custom)).toContain(
      '"remaining_path": "balance"'
    )
    expect(stringifyBalanceQueryConfig(custom)).toContain(
      '"url": "https://api.example.com/v1/user/balance"'
    )
    expect(stringifyBalanceQueryConfig({ mode: 'auto' })).toContain(
      '"mode": "auto"'
    )
  })

  test('defaults unconfigured channels to disabled and validates automatic alerts', () => {
    expect(parseBalanceQueryConfig('').mode).toBe('disabled')

    const config = createBalanceQueryConfig('new_api')
    config.auth = {
      type: 'header',
      name: 'Authorization',
      value: 'account-token',
    }
    config.auto_refresh = true
    config.refresh_minutes = 15
    config.low_balance_alert = true
    config.low_balance_threshold = '10'
    expect(validateBalanceQueryConfig(config)).toBeNull()

    config.low_balance_threshold = '0'
    expect(validateBalanceQueryConfig(config)).toBe(
      'Low balance threshold must be a positive number'
    )
  })

  test('supports OpenModel-style total minus frozen mapping and explicit methods', () => {
    const custom = createBalanceQueryConfig('custom')
    custom.url = 'https://api.openmodel.ai/web/v1/self'
    custom.method = 'GET'
    custom.auth = {
      type: 'header',
      name: 'Authorization',
      value: 'Bearer account-token',
    }
    custom.remaining_mode = 'total_minus_used'
    custom.response = {
      total_path: 'balance',
      used_path: 'frozen_balance',
    }
    custom.multiplier = '0.000001'

    expect(validateBalanceQueryConfig(custom)).toBeNull()
    const serialized = stringifyBalanceQueryConfig(custom)
    expect(serialized).toContain('"method": "GET"')
    expect(serialized).toContain('"remaining_mode": "total_minus_used"')

    custom.method = 'POST'
    custom.body = '{invalid'
    expect(validateBalanceQueryConfig(custom)).toBe(
      'POST request body must be valid JSON'
    )
    custom.body = '{"scope":"wallet"}'
    expect(validateBalanceQueryConfig(custom)).toBeNull()
  })

  test('formats structured balances in their native unit', () => {
    expect(
      formatChannelBalanceInfo({
        remaining: '55.002146',
        unit: 'money',
        currency: 'USD',
        display_unit: '$',
        metric_kind: 'wallet',
        source: 'custom',
        unlimited: false,
        updated_at: 1,
      })
    ).toBe('$55.0021')
    expect(
      formatChannelBalanceInfo(
        {
          unit: 'credits',
          unlimited: true,
          updated_at: 1,
        },
        { unlimitedLabel: '无限额度' }
      )
    ).toBe('无限额度')
  })

  test('falls back safely when the active language tag is not Intl-compatible', () => {
    expect(
      formatChannelBalanceInfo(
        {
          remaining: '2.5',
          unit: 'money',
          currency: 'USD',
          display_unit: '$',
          unlimited: false,
          updated_at: 1,
        },
        { locale: 'zhCN' }
      )
    ).toBe('$2.5')
  })
})

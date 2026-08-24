/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import type {
  AdvancedCustomAuthType,
  ChannelBalanceMetricKind,
  ChannelBalanceQueryConfig,
  ChannelBalanceQueryMode,
  ChannelBalanceUnit,
} from '../types'

export const CHANNEL_TYPE_SUB2_API = 59
export const CHANNEL_TYPE_NEW_API = 60

export const BALANCE_QUERY_MODE_OPTIONS: Array<{
  value: ChannelBalanceQueryMode
  label: string
  description: string
}> = [
  {
    value: 'auto',
    label: 'Follow channel type',
    description: 'Use the built-in balance adapter for this channel type.',
  },
  {
    value: 'disabled',
    label: 'Disabled',
    description: 'Do not query or automatically update upstream balance.',
  },
  {
    value: 'new_api',
    label: 'NewAPI compatible',
    description:
      'Query /api/user/self with an account access token to read the account wallet balance.',
  },
  {
    value: 'one_api',
    label: 'OneAPI compatible',
    description: 'Query /api/user/self with an account access token.',
  },
  {
    value: 'sub2api',
    label: 'Sub2API compatible',
    description:
      'Query /v1/usage and preserve wallet, quota, or subscription meaning.',
  },
  {
    value: 'gcp_trial_credit',
    label: 'Vertex trial credit balance',
    description:
      'Reuse a Vertex service account JSON and calculate the remaining Google Cloud trial credit from the standard BigQuery billing export.',
  },
  {
    value: 'custom',
    label: 'Custom mapping',
    description:
      'Manually enter the complete request URL, authentication, JSON response fields, unit, currency, and multiplier.',
  },
]

export const BALANCE_QUERY_AUTH_OPTIONS: Array<{
  value: AdvancedCustomAuthType
  label: string
}> = [
  { value: 'header', label: 'Header' },
  { value: 'query', label: 'Query parameter' },
  { value: 'none', label: 'No authentication' },
]

export const BALANCE_QUERY_UNIT_OPTIONS: Array<{
  value: ChannelBalanceUnit
  label: string
}> = [
  { value: 'money', label: 'Money' },
  { value: 'credits', label: 'Credits' },
  { value: 'tokens', label: 'Tokens' },
  { value: 'requests', label: 'Requests' },
]

export const BALANCE_QUERY_METRIC_OPTIONS: Array<{
  value: ChannelBalanceMetricKind
  label: string
}> = [
  { value: 'wallet', label: 'Wallet balance' },
  { value: 'quota', label: 'Quota' },
  { value: 'subscription', label: 'Subscription allowance' },
  { value: 'rate_limit', label: 'Rate limit' },
  { value: 'custom', label: 'Custom metric' },
]

export function createBalanceQueryConfig(
  mode: ChannelBalanceQueryMode
): ChannelBalanceQueryConfig {
  if (mode === 'custom') {
    return {
      mode,
      url: '',
      path: '',
      method: 'GET',
      body: '',
      auth: {
        type: 'header',
        name: '',
        value: '',
      },
      headers: [],
      response: {
        remaining_path: '',
        total_path: '',
        used_path: '',
        currency_path: '',
        active_path: '',
        unlimited_path: '',
        success_path: '',
        success_value: '',
      },
      unit: 'money',
      currency: '',
      display_unit: '',
      metric_kind: 'custom',
      multiplier: '1',
      remaining_mode: 'direct',
    }
  }
  if (mode === 'new_api' || mode === 'one_api') {
    return {
      mode,
      auth: {
        type: 'header',
        name: 'Authorization',
        value: '',
      },
      account_user_id: '',
    }
  }
  if (mode === 'gcp_trial_credit') {
    return {
      mode,
      gcp_trial: {
        billing_account_id: '',
        query_project_id: '',
        dataset_id: 'billing_export',
        credential_channel_id: 0,
        total_amount: '300',
        baseline_used: '132',
        baseline_at: Math.floor(Date.now() / 1000),
      },
    }
  }
  return { mode }
}

export function parseBalanceQueryConfig(
  value: string | undefined
): ChannelBalanceQueryConfig {
  if (!value?.trim()) return createBalanceQueryConfig('disabled')
  try {
    const parsed = JSON.parse(value) as ChannelBalanceQueryConfig
    return normalizeBalanceQueryConfig(parsed)
  } catch {
    return createBalanceQueryConfig('auto')
  }
}

export function stringifyBalanceQueryConfig(
  config: ChannelBalanceQueryConfig
): string {
  const normalized = normalizeBalanceQueryConfig(config)
  return JSON.stringify(normalized, null, 2)
}

export function normalizeBalanceQueryConfig(
  config: ChannelBalanceQueryConfig
): ChannelBalanceQueryConfig {
  const mode = config.mode || 'auto'
  const automation = {
    auto_refresh: config.auto_refresh === true,
    refresh_minutes: Math.min(
      1440,
      Math.max(1, Math.trunc(Number(config.refresh_minutes || 15)))
    ),
    low_balance_alert: config.low_balance_alert === true,
    low_balance_threshold: config.low_balance_threshold?.trim() || '',
  }
  if (mode !== 'custom') {
    if (mode === 'auto') {
      return {
        mode,
        ...automation,
        auth: config.auth
          ? {
              type: 'header',
              name: 'Authorization',
              value: config.auth.value?.trim() || '',
            }
          : undefined,
        auth_configured: config.auth_configured === true,
        auth_masked:
          config.auth_configured === true
            ? config.auth_masked?.trim() || ''
            : '',
        account_user_id: config.account_user_id?.trim() || '',
      }
    }
    if (mode === 'new_api' || mode === 'one_api') {
      return {
        mode,
        ...automation,
        auth: {
          type: 'header',
          name: 'Authorization',
          value: config.auth?.value?.trim() || '',
        },
        auth_configured: config.auth_configured === true,
        auth_masked:
          config.auth_configured === true
            ? config.auth_masked?.trim() || ''
            : '',
        account_user_id: config.account_user_id?.trim() || '',
      }
    }
    if (mode === 'gcp_trial_credit') {
      return {
        mode,
        ...automation,
        gcp_trial: {
          billing_account_id:
            config.gcp_trial?.billing_account_id?.trim().toUpperCase() || '',
          query_project_id: config.gcp_trial?.query_project_id?.trim() || '',
          dataset_id: config.gcp_trial?.dataset_id?.trim() || '',
          credential_channel_id: Math.max(
            0,
            Math.trunc(Number(config.gcp_trial?.credential_channel_id || 0))
          ),
          total_amount: config.gcp_trial?.total_amount?.trim() || '300',
          baseline_used: config.gcp_trial?.baseline_used?.trim() || '0',
          baseline_at:
            Math.trunc(Number(config.gcp_trial?.baseline_at || 0)) || 0,
        },
      }
    }
    return { mode, ...automation }
  }
  const authType = config.auth?.type || 'header'
  const auth =
    authType === 'none'
      ? { type: 'none' as const }
      : {
          type: authType,
          name: config.auth?.name?.trim() || '',
          value: config.auth?.value || '',
        }
  return {
    mode,
    ...automation,
    url: config.url?.trim() || '',
    path: config.path?.trim() || '',
    method: config.method?.trim().toUpperCase() === 'POST' ? 'POST' : 'GET',
    body: config.body || '',
    auth,
    headers: (config.headers || []).map((header) => ({
      name: header.name?.trim() || '',
      value: header.value || '',
      configured: header.configured === true,
      masked: header.configured === true ? header.masked?.trim() || '' : '',
    })),
    response: {
      remaining_path: config.response?.remaining_path?.trim() || '',
      total_path: config.response?.total_path?.trim() || '',
      used_path: config.response?.used_path?.trim() || '',
      currency_path: config.response?.currency_path?.trim() || '',
      active_path: config.response?.active_path?.trim() || '',
      unlimited_path: config.response?.unlimited_path?.trim() || '',
      success_path: config.response?.success_path?.trim() || '',
      success_value: config.response?.success_value?.trim() || '',
    },
    auth_configured: config.auth_configured === true,
    auth_masked:
      config.auth_configured === true ? config.auth_masked?.trim() || '' : '',
    unit: config.unit || 'money',
    currency: config.currency?.trim().toUpperCase() || '',
    display_unit: config.display_unit?.trim() || '',
    metric_kind: config.metric_kind || 'custom',
    multiplier: config.multiplier?.trim() || '1',
    remaining_mode:
      config.remaining_mode === 'total_minus_used'
        ? 'total_minus_used'
        : 'direct',
  }
}

export function validateBalanceQueryConfig(
  config: ChannelBalanceQueryConfig
): string | null {
  const normalized = normalizeBalanceQueryConfig(config)
  if (normalized.auto_refresh) {
    if (normalized.mode === 'disabled') {
      return 'Automatic refresh cannot be enabled while balance query is disabled'
    }
    if (
      !Number.isInteger(normalized.refresh_minutes) ||
      Number(normalized.refresh_minutes) < 1 ||
      Number(normalized.refresh_minutes) > 1440
    ) {
      return 'Balance refresh interval must be between 1 and 1440 minutes'
    }
  }
  if (normalized.low_balance_alert) {
    if (!normalized.auto_refresh) {
      return 'Low balance alert requires automatic refresh'
    }
    const threshold = Number(normalized.low_balance_threshold || '0')
    if (!Number.isFinite(threshold) || threshold <= 0) {
      return 'Low balance threshold must be a positive number'
    }
  }
  if (normalized.mode === 'new_api' || normalized.mode === 'one_api') {
    const value = normalized.auth?.value?.trim() || ''
    if (
      (!value && normalized.auth_configured !== true) ||
      value.includes('{api_key}')
    ) {
      return 'Account access token is required; the channel API key cannot read the account wallet balance'
    }
    const accountUserID = normalized.account_user_id?.trim() || ''
    if (accountUserID && !/^[1-9]\d*$/.test(accountUserID)) {
      return 'Upstream account user ID must be a positive integer'
    }
    return null
  }
  if (normalized.mode === 'gcp_trial_credit') {
    const gcp = normalized.gcp_trial
    if (
      !gcp ||
      !/^[A-Z0-9]{6}-[A-Z0-9]{6}-[A-Z0-9]{6}$/.test(
        gcp.billing_account_id || ''
      )
    ) {
      return 'Google Cloud billing account ID is required'
    }
    if (!/^[a-z][a-z0-9-]{4,28}[a-z0-9]$/.test(gcp.query_project_id || '')) {
      return 'BigQuery project ID is invalid'
    }
    if (!/^[A-Za-z_][A-Za-z0-9_]{0,1023}$/.test(gcp.dataset_id || '')) {
      return 'BigQuery dataset ID is invalid'
    }
    if (
      !Number.isInteger(Number(gcp.credential_channel_id || 0)) ||
      Number(gcp.credential_channel_id || 0) < 0
    ) {
      return 'Vertex credential channel ID must be a positive integer'
    }
    const total = Number(gcp.total_amount || '0')
    const baseline = Number(gcp.baseline_used || '0')
    if (!Number.isFinite(total) || total <= 0) {
      return 'Trial credit total must be a positive number'
    }
    if (!Number.isFinite(baseline) || baseline < 0 || baseline > total) {
      return 'Historical used baseline must be between zero and the total'
    }
    if (
      !Number.isInteger(Number(gcp.baseline_at || 0)) ||
      Number(gcp.baseline_at) <= 0
    ) {
      return 'Tracking baseline time is required'
    }
    return null
  }
  if (normalized.mode !== 'custom') return null
  const requestURL = normalized.url || ''
  if (requestURL) {
    try {
      const parsed = new URL(requestURL)
      if (
        !['http:', 'https:'].includes(parsed.protocol) ||
        !parsed.hostname ||
        parsed.username ||
        parsed.password ||
        parsed.hash
      ) {
        return 'Balance query URL must be an absolute HTTP or HTTPS URL'
      }
    } catch {
      return 'Balance query URL must be an absolute HTTP or HTTPS URL'
    }
  } else {
    const path = normalized.path || ''
    if (!path.startsWith('/') || path.startsWith('//')) {
      return 'Balance query URL is required'
    }
  }
  if (normalized.method === 'GET' && normalized.body?.trim()) {
    return 'GET balance queries cannot include a request body'
  }
  if (normalized.method === 'POST' && normalized.body?.trim()) {
    try {
      JSON.parse(normalized.body)
    } catch {
      return 'POST request body must be valid JSON'
    }
  }
  if (normalized.remaining_mode === 'total_minus_used') {
    if (!normalized.response?.total_path || !normalized.response?.used_path) {
      return 'Total and used JSON paths are required for total minus used'
    }
  } else if (!normalized.response?.remaining_path) {
    return 'Remaining balance JSON path is required'
  }
  if (normalized.response?.success_path && !normalized.response.success_value) {
    return 'Success value is required when a success path is configured'
  }
  const multiplier = Number(normalized.multiplier || '1')
  if (!Number.isFinite(multiplier) || multiplier <= 0) {
    return 'Balance multiplier must be a positive number'
  }
  if (normalized.auth?.type !== 'none' && !normalized.auth?.name?.trim()) {
    return 'Authentication name is required'
  }
  if (
    normalized.auth?.type !== 'none' &&
    !normalized.auth?.value?.trim() &&
    normalized.auth_configured !== true
  ) {
    return 'Authentication value is required'
  }
  if ((normalized.headers?.length || 0) > 20) {
    return 'At most 20 additional headers are supported'
  }
  const headerNames = new Set<string>()
  if (normalized.auth?.type === 'header' && normalized.auth.name?.trim()) {
    headerNames.add(normalized.auth.name.trim().toLowerCase())
  }
  for (const header of normalized.headers || []) {
    const name = header.name?.trim() || ''
    if (!name || /[\s:]/.test(name)) {
      return 'Additional header name is invalid'
    }
    const normalizedName = name.toLowerCase()
    if (headerNames.has(normalizedName)) {
      return 'Additional header names must be unique'
    }
    headerNames.add(normalizedName)
    if (!header.value?.trim() && header.configured !== true) {
      return 'Additional header value is required'
    }
    if (/[\r\n]/.test(header.value || '')) {
      return 'Additional header value is invalid'
    }
  }
  return null
}

export function getEffectiveBalanceQueryPath(
  config: ChannelBalanceQueryConfig,
  channelType: number
): string | null {
  let mode = config.mode || 'auto'
  if (mode === 'auto') {
    if (channelType === CHANNEL_TYPE_NEW_API) mode = 'new_api'
    if (channelType === CHANNEL_TYPE_SUB2_API) mode = 'sub2api'
  }
  switch (mode) {
    case 'new_api':
      return '/api/user/self'
    case 'one_api':
      return '/api/user/self'
    case 'sub2api':
      return '/v1/usage'
    case 'gcp_trial_credit':
      return null
    case 'custom':
      return config.path?.trim() || null
    default:
      return null
  }
}

export function getGCPTrialBillingTable(
  config: ChannelBalanceQueryConfig
): string {
  const gcp = config.gcp_trial
  if (!gcp?.query_project_id || !gcp.dataset_id || !gcp.billing_account_id) {
    return ''
  }
  return `${gcp.query_project_id}.${gcp.dataset_id}.gcp_billing_export_v1_${gcp.billing_account_id.replaceAll('-', '_')}`
}

export function buildBalanceQueryPreviewURL(
  baseURL: string | undefined,
  path: string | null
): string {
  const base = String(baseURL || '')
    .trim()
    .replace(/\/+$/, '')
  if (!base || !path) return ''
  return `${base}/${path.replace(/^\/+/, '')}`
}

export function getBalanceQueryModeLabel(
  config: ChannelBalanceQueryConfig,
  channelType: number
): string {
  const mode = config.mode || 'auto'
  if (mode === 'auto' && channelType === CHANNEL_TYPE_NEW_API) {
    if (config.auth?.value?.trim() || config.auth_configured === true) {
      return 'NewAPI account balance'
    }
    return 'NewAPI account token required'
  }
  if (mode === 'auto' && channelType === CHANNEL_TYPE_SUB2_API) {
    return 'Sub2API built-in'
  }
  return (
    BALANCE_QUERY_MODE_OPTIONS.find((option) => option.value === mode)?.label ||
    'Follow channel type'
  )
}

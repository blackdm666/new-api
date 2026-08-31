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
export type TurnstileProvider = 'cloudflare' | 'custom'

export type TurnstileClientConfig = {
  provider: TurnstileProvider
  siteKey: string
  widgetScriptURL: string
  widgetEndpoint: string
  action: string
}

type TurnstileStatusFields = {
  turnstile_provider?: TurnstileProvider
  turnstile_site_key?: string
  turnstile_widget_script_url?: string
  turnstile_widget_endpoint?: string
  turnstile_action?: string
}

type TurnstileStatusLike = TurnstileStatusFields & {
  data?: TurnstileStatusFields
}

export function getTurnstileClientConfig(
  status?: TurnstileStatusLike | null
): TurnstileClientConfig {
  const source = status?.data ?? status
  return {
    provider: source?.turnstile_provider === 'custom' ? 'custom' : 'cloudflare',
    siteKey: source?.turnstile_site_key?.trim() || '',
    widgetScriptURL: source?.turnstile_widget_script_url?.trim() || '',
    widgetEndpoint: source?.turnstile_widget_endpoint?.trim() || '',
    action: source?.turnstile_action?.trim() || 'register',
  }
}

export function isTurnstileClientConfigured(
  config: TurnstileClientConfig
): boolean {
  if (config.provider === 'custom') {
    return Boolean(config.widgetScriptURL && config.widgetEndpoint)
  }
  return Boolean(config.siteKey)
}

export function resolveTurnstileWidgetEndpoint(
  endpoint: string,
  useDevelopmentProxy: boolean
): string {
  return useDevelopmentProxy ? '/__captcha' : endpoint
}

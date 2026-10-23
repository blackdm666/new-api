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

// CC Switch (v4.0.10+) shows a provider's balance on its card from a usage
// query script carried in the import link. The script runs in CC Switch's
// sandbox and may send one HTTPS request to the provider's own origin, so it
// calls /api/usage/token/ with the imported key. That endpoint reports the
// owner's wallet only when the key allows user quota queries; otherwise it
// reports the key's own quota, which is only meaningful for a limited key.

/** Minutes between background balance refreshes in CC Switch. */
export const CC_SWITCH_USAGE_INTERVAL_MINUTES = 30

export interface CCSwitchUsageLabels {
  wallet: string
  key: string
  disabled: string
  failed: string
}

export interface CCSwitchUsageUnit {
  /** Quota per displayed currency unit, e.g. 500000 per ¥1. */
  quotaPerUnit: number
  /** Unit text CC Switch prints after the amount, e.g. CNY. */
  unit: string
}

/**
 * Builds the CC Switch usage query script. It is plain ES5 because CC Switch
 * evaluates it in QuickJS; {{baseUrl}} and {{apiKey}} are filled in by CC
 * Switch. The base URL may end in /v1 (Codex, Grok Build), which is removed
 * so every app reaches the same endpoint.
 */
export function buildCCSwitchUsageScript(
  unit: CCSwitchUsageUnit,
  labels: CCSwitchUsageLabels
): string {
  return `({
  request: {
    url: "{{baseUrl}}".replace(/\\/+$/, "").replace(/\\/v1$/, "") + "/api/usage/token/",
    method: "GET",
    headers: { "Authorization": "Bearer {{apiKey}}", "User-Agent": "cc-switch/1.0" }
  },
  extractor: function (response) {
    var perUnit = ${JSON.stringify(unit.quotaPerUnit)};
    var unit = ${JSON.stringify(unit.unit)};
    var data = response && response.data;
    if (!response || response.code !== true || !data) {
      return { isValid: false, invalidMessage: (response && response.message) || ${JSON.stringify(labels.failed)} };
    }
    var lines = [];
    if (data.user_quota) {
      lines.push({ planName: ${JSON.stringify(labels.wallet)}, remaining: data.user_quota.total_available / perUnit, unit: unit });
    }
    if (!data.unlimited_quota) {
      lines.push({ planName: ${JSON.stringify(labels.key)}, remaining: data.total_available / perUnit, used: data.total_used / perUnit, total: data.total_granted / perUnit, unit: unit });
    }
    if (lines.length === 0) {
      return { isValid: true, extra: ${JSON.stringify(labels.disabled)} };
    }
    return lines.length === 1 ? lines[0] : lines;
  }
})`
}

/** Base64 of the UTF-8 script, the encoding CC Switch expects in links. */
export function encodeCCSwitchUsageScript(script: string): string {
  const bytes = new TextEncoder().encode(script)
  let binary = ''
  for (const byte of bytes) binary += String.fromCharCode(byte)
  return btoa(binary)
}

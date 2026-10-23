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
import { describe, expect, test } from 'vitest'

import {
  buildCCSwitchUsageScript,
  encodeCCSwitchUsageScript,
} from '../cc-switch-usage'

const labels = {
  wallet: '钱包余额',
  key: 'Key 额度',
  disabled: 'turn it on',
  failed: 'query failed',
}

type UsageLine = Record<string, unknown>
type UsageScript = {
  request: { url: string; method: string; headers: Record<string, string> }
  extractor: (response: unknown) => UsageLine | UsageLine[]
}

// Evaluates the script the way CC Switch does: placeholders are replaced
// first, then the expression is run and its extractor called on the body.
function runScript(baseUrl: string): UsageScript {
  const script = buildCCSwitchUsageScript(
    { quotaPerUnit: 500000, unit: 'CNY' },
    labels
  )
    .replaceAll('{{baseUrl}}', baseUrl)
    .replaceAll('{{apiKey}}', 'sk-test')
  return new Function(`return ${script}`)() as UsageScript
}

function tokenUsage(data: Record<string, unknown>) {
  return { code: true, message: 'ok', data }
}

describe('CC Switch usage script request', () => {
  test.each([
    ['https://api.88api.ai', 'https://api.88api.ai/api/usage/token/'],
    ['https://api.88api.ai/v1', 'https://api.88api.ai/api/usage/token/'],
    ['https://api.88api.ai/v1/', 'https://api.88api.ai/api/usage/token/'],
  ])(
    'base URL %s queries the token usage endpoint on its own origin',
    (base, url) => {
      const request = runScript(base).request
      expect(request.url).toBe(url)
      expect(request.method).toBe('GET')
      expect(request.headers.Authorization).toBe('Bearer sk-test')
    }
  )
})

describe('CC Switch usage script extractor', () => {
  const extractor = runScript('https://api.88api.ai').extractor

  test('an unlimited key that may read the account shows only the wallet balance', () => {
    const result = extractor(
      tokenUsage({
        unlimited_quota: true,
        total_available: -14386383,
        user_quota: { total_available: 61725000, total_used: 1000 },
      })
    )
    expect(result).toEqual({
      planName: '钱包余额',
      remaining: 123.45,
      unit: 'CNY',
    })
  })

  test('a limited key that may read the account shows the wallet and the key quota', () => {
    const result = extractor(
      tokenUsage({
        unlimited_quota: false,
        total_available: 2500000,
        total_used: 2500000,
        total_granted: 5000000,
        user_quota: { total_available: 500000, total_used: 0 },
      })
    )
    expect(result).toEqual([
      { planName: '钱包余额', remaining: 1, unit: 'CNY' },
      { planName: 'Key 额度', remaining: 5, used: 5, total: 10, unit: 'CNY' },
    ])
  })

  test('a limited key without account access shows its own quota', () => {
    const result = extractor(
      tokenUsage({
        unlimited_quota: false,
        total_available: 500000,
        total_used: 0,
        total_granted: 500000,
      })
    )
    expect(result).toEqual({
      planName: 'Key 额度',
      remaining: 1,
      used: 0,
      total: 1,
      unit: 'CNY',
    })
  })

  test('an unlimited key without account access explains how to show the balance', () => {
    const result = extractor(
      tokenUsage({ unlimited_quota: true, total_available: -100 })
    )
    expect(result).toEqual({ isValid: true, extra: 'turn it on' })
  })

  test('a failed lookup is reported with the server message', () => {
    expect(extractor({ success: false, message: 'token not found' })).toEqual({
      isValid: false,
      invalidMessage: 'token not found',
    })
    expect(extractor(null)).toEqual({
      isValid: false,
      invalidMessage: 'query failed',
    })
  })
})

describe('encodeCCSwitchUsageScript', () => {
  test('non-ASCII labels survive the Base64 round trip as UTF-8', () => {
    const script = buildCCSwitchUsageScript(
      { quotaPerUnit: 500000, unit: 'CNY' },
      labels
    )
    const encoded = encodeCCSwitchUsageScript(script)
    const decoded = new TextDecoder().decode(
      Uint8Array.from(atob(encoded), (char) => char.charCodeAt(0))
    )
    expect(decoded).toBe(script)
    expect(decoded).toContain('钱包余额')
  })
})

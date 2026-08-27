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
import * as z from 'zod'

import { combineBillingExpr } from '@/features/pricing/lib/billing-expr'

import { formatPricingNumber } from './pricing-format'

const optionalPricingNumber = (t: (key: string) => string) =>
  z
    .string()
    .optional()
    .refine(
      (value) =>
        value === undefined || value === '' || isCompletePricingNumber(value),
      t('Please enter a valid number')
    )

export const createModelPricingSchema = (t: (key: string) => string) => {
  const pricingNumber = optionalPricingNumber(t)
  return z.object({
    name: z.string().min(1, t('Model name is required')),
    price: pricingNumber,
    ratio: pricingNumber,
    cacheRatio: pricingNumber,
    createCacheRatio: pricingNumber,
    completionRatio: pricingNumber,
    imageRatio: pricingNumber,
    audioRatio: pricingNumber,
    audioCompletionRatio: pricingNumber,
  })
}

export type ModelPricingFormValues = z.infer<
  ReturnType<typeof createModelPricingSchema>
>

export type PricingMode =
  | 'per-token'
  | 'per-request'
  | 'per-second'
  | 'tiered_expr'

export type LaneKey =
  | 'completion'
  | 'cache'
  | 'createCache'
  | 'image'
  | 'audioInput'
  | 'audioOutput'

export type ModelRatioData = {
  name: string
  price?: string
  ratio?: string
  cacheRatio?: string
  createCacheRatio?: string
  completionRatio?: string
  imageRatio?: string
  audioRatio?: string
  audioCompletionRatio?: string
  billingMode?: PricingMode
  billingExpr?: string
  requestRuleExpr?: string
}

export type PreviewRow = {
  key: string
  label: string
  value: string
  multiline?: boolean
}

export const numericDraftRegex = /^(\d+(\.\d*)?|\.\d*)?([eE][+-]?\d*)?$/
const completePricingNumberRegex = /^(\d+(\.\d*)?|\.\d+)([eE][+-]?\d+)?$/
const zeroPricingNumberRegex = /^(0+(\.0*)?|\.0+)([eE][+-]?\d+)?$/

export function isCompletePricingNumber(value: unknown): boolean {
  if (typeof value !== 'string' && typeof value !== 'number') return false
  const text = String(value)
  if (!completePricingNumberRegex.test(text)) return false
  const numeric = Number(text)
  if (!Number.isFinite(numeric) || numeric < 0) return false
  return numeric !== 0 || zeroPricingNumberRegex.test(text)
}

export const EMPTY_LANE_PRICES: Record<LaneKey, string> = {
  completion: '',
  cache: '',
  createCache: '',
  image: '',
  audioInput: '',
  audioOutput: '',
}

export const EMPTY_LANE_ENABLED: Record<LaneKey, boolean> = {
  completion: false,
  cache: false,
  createCache: false,
  image: false,
  audioInput: false,
  audioOutput: false,
}

export const ratioFieldByLane: Record<LaneKey, keyof ModelPricingFormValues> = {
  completion: 'completionRatio',
  cache: 'cacheRatio',
  createCache: 'createCacheRatio',
  image: 'imageRatio',
  audioInput: 'audioRatio',
  audioOutput: 'audioCompletionRatio',
}

export const laneConfigs: Array<{
  key: LaneKey
  titleKey: string
  descriptionKey: string
  placeholder: string
}> = [
  {
    key: 'completion',
    titleKey: 'Completion price',
    descriptionKey: 'Output token price for generated tokens.',
    placeholder: '15',
  },
  {
    key: 'cache',
    titleKey: 'Cache read price',
    descriptionKey: 'Token price for cache reads.',
    placeholder: '0.3',
  },
  {
    key: 'createCache',
    titleKey: 'Cache write price',
    descriptionKey: 'Token price for creating cache entries.',
    placeholder: '3.75',
  },
  {
    key: 'image',
    titleKey: 'Image input price',
    descriptionKey: 'Token price for image input.',
    placeholder: '2.5',
  },
  {
    key: 'audioInput',
    titleKey: 'Audio input price',
    descriptionKey: 'Token price for audio input.',
    placeholder: '3.81',
  },
  {
    key: 'audioOutput',
    titleKey: 'Audio output price',
    descriptionKey: 'Token price for audio output.',
    placeholder: '15.11',
  },
]

export function hasValue(value: unknown): boolean {
  return (
    value !== '' && value !== null && value !== undefined && value !== false
  )
}

export function toNumberOrNull(value: unknown): number | null {
  if (!hasValue(value) && value !== 0) return null
  const num = Number(value)
  return Number.isFinite(num) ? num : null
}

function ratioToBasePrice(ratio: unknown, exchangeRate = 1): string {
  const num = toNumberOrNull(ratio)
  if (num === null) return ''
  return formatPricingNumber(num * 2 * exchangeRate)
}

function deriveLanePrice(
  ratio: unknown,
  denominator: unknown,
  fallback = ''
): string {
  const ratioNumber = toNumberOrNull(ratio)
  const denominatorNumber = toNumberOrNull(denominator)
  if (ratioNumber === null || denominatorNumber === null) return fallback
  return formatPricingNumber(ratioNumber * denominatorNumber)
}

export function createInitialLaneState(
  data?: ModelRatioData | null,
  exchangeRate = 1
) {
  if (!data) {
    return {
      promptPrice: '',
      prices: { ...EMPTY_LANE_PRICES },
      enabled: { ...EMPTY_LANE_ENABLED },
    }
  }

  const promptPrice = ratioToBasePrice(data.ratio, exchangeRate)
  const audioInputPrice = deriveLanePrice(data.audioRatio, promptPrice)
  const prices: Record<LaneKey, string> = {
    completion: deriveLanePrice(data.completionRatio, promptPrice),
    cache: deriveLanePrice(data.cacheRatio, promptPrice),
    createCache: deriveLanePrice(data.createCacheRatio, promptPrice),
    image: deriveLanePrice(data.imageRatio, promptPrice),
    audioInput: audioInputPrice,
    audioOutput: deriveLanePrice(data.audioCompletionRatio, audioInputPrice),
  }

  return {
    promptPrice,
    prices,
    enabled: {
      completion: hasValue(data.completionRatio),
      cache: hasValue(data.cacheRatio),
      createCache: hasValue(data.createCacheRatio),
      image: hasValue(data.imageRatio),
      audioInput: hasValue(data.audioRatio),
      audioOutput: hasValue(data.audioCompletionRatio),
    },
  }
}

export function buildPreviewRows(
  values: ModelPricingFormValues,
  mode: PricingMode,
  billingExpr: string,
  requestRuleExpr: string,
  promptPrice: string,
  lanePrices: Record<LaneKey, string>,
  laneEnabled: Record<LaneKey, boolean>,
  t: (key: string) => string,
  currencySymbol = '$'
): PreviewRow[] {
  if (mode === 'tiered_expr') {
    const effectiveExpr = combineBillingExpr(billingExpr, requestRuleExpr)
    return [
      { key: 'mode', label: 'BillingMode', value: 'tiered_expr' },
      {
        key: 'expr',
        label: t('Expression'),
        value: effectiveExpr || t('Empty'),
        multiline: true,
      },
    ]
  }

  if (mode === 'per-request' || mode === 'per-second') {
    return [
      {
        key: 'mode',
        label: 'BillingMode',
        value: mode === 'per-second' ? 'per_second' : 'per_request',
      },
      {
        key: 'price',
        label: 'ModelPrice',
        value: values.price ? `${currencySymbol}${values.price}` : t('Empty'),
      },
    ]
  }

  return [
    {
      key: 'inputPrice',
      label: t('Input price'),
      value: promptPrice ? `${currencySymbol}${promptPrice}` : t('Empty'),
    },
    {
      key: 'completion',
      label: t('Completion price'),
      value:
        laneEnabled.completion && lanePrices.completion
          ? `${currencySymbol}${lanePrices.completion}`
          : t('Empty'),
    },
    {
      key: 'cache',
      label: t('Cache read price'),
      value:
        laneEnabled.cache && lanePrices.cache
          ? `${currencySymbol}${lanePrices.cache}`
          : t('Empty'),
    },
    {
      key: 'createCache',
      label: t('Cache write price'),
      value:
        laneEnabled.createCache && lanePrices.createCache
          ? `${currencySymbol}${lanePrices.createCache}`
          : t('Empty'),
    },
    {
      key: 'image',
      label: t('Image input price'),
      value:
        laneEnabled.image && lanePrices.image
          ? `${currencySymbol}${lanePrices.image}`
          : t('Empty'),
    },
    {
      key: 'audio',
      label: t('Audio input price'),
      value:
        laneEnabled.audioInput && lanePrices.audioInput
          ? `${currencySymbol}${lanePrices.audioInput}`
          : t('Empty'),
    },
    {
      key: 'audioCompletion',
      label: t('Audio output price'),
      value:
        laneEnabled.audioOutput && lanePrices.audioOutput
          ? `${currencySymbol}${lanePrices.audioOutput}`
          : t('Empty'),
    },
  ]
}

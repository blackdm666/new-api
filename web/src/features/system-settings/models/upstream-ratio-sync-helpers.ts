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
import type { RatioType } from '../types'
import {
  MODELS_DEV_PRESET_ID,
  MODELS_DEV_PRESET_NAME,
  OFFICIAL_CHANNEL_ID,
  OFFICIAL_CHANNEL_NAME,
  RATIO_TYPE_OPTIONS,
} from './constants'
import { isCompletePricingNumber } from './model-pricing-core'

export type RatioDifferenceEntry = {
  current: number | string | null
  upstreams: Record<string, number | string | 'same'>
  confidence: Record<string, boolean>
}

export type ModelRow = {
  key: string
  model: string
  ratioTypes: Partial<Record<RatioType, RatioDifferenceEntry>>
  billingConflict: boolean
}

export type ResolutionsMap = Record<string, Record<string, number | string>>

export type PricingOptionMaps = {
  ModelRatio: Record<string, number>
  CompletionRatio: Record<string, number>
  CacheRatio: Record<string, number>
  CreateCacheRatio: Record<string, number>
  ImageRatio: Record<string, number>
  AudioRatio: Record<string, number>
  AudioCompletionRatio: Record<string, number>
  ModelPrice: Record<string, number>
  'billing_setting.billing_mode': Record<string, string>
  'billing_setting.billing_expr': Record<string, string>
}

export type ResolutionSelection = {
  model: string
  ratioType: RatioType
  value: number | string
  sourceName: string
}

export type ResolvedResolutionSelection = ResolutionSelection & {
  ratioType: RatioType
}

export type ResolutionRemoval = {
  model: string
  ratioType: RatioType
}

export type ResolutionRemovalPlan = Map<string, Set<RatioType>>

export const RATIO_SYNC_FIELDS: RatioType[] = [
  'model_ratio',
  'completion_ratio',
  'cache_ratio',
  'create_cache_ratio',
  'image_ratio',
  'audio_ratio',
  'audio_completion_ratio',
]

export const SYNC_FIELD_ORDER: RatioType[] = [
  ...RATIO_SYNC_FIELDS,
  'model_price',
  'billing_mode',
  'billing_expr',
]

export const NUMERIC_SYNC_FIELDS = new Set<string>([
  ...RATIO_SYNC_FIELDS,
  'model_price',
])

export function getSyncFieldLabel(
  ratioType: string,
  t: (key: string) => string
): string {
  const opt = RATIO_TYPE_OPTIONS.find((o) => o.value === ratioType)
  if (opt) return t(opt.label)
  return ratioType
}

export function getOrderedRatioTypes(
  ratioTypes: Partial<Record<RatioType, RatioDifferenceEntry>>,
  filter?: string
): RatioType[] {
  const keys = Object.keys(ratioTypes) as RatioType[]
  const ordered = [
    ...SYNC_FIELD_ORDER.filter((f) => keys.includes(f)),
    ...keys.filter((f) => !SYNC_FIELD_ORDER.includes(f)),
  ]
  if (!filter || filter === '__all__') return ordered
  return ordered.filter((f) => f === filter)
}

export function getPreferredSyncField(
  ratioTypes: Partial<Record<RatioType, RatioDifferenceEntry>>,
  ratioType: RatioType,
  sourceName: string
): RatioType {
  const billingModeValue = ratioTypes.billing_mode?.upstreams?.[sourceName]
  const fixedPriceValue = ratioTypes.model_price?.upstreams?.[sourceName]
  if (
    ratioType === 'billing_mode' &&
    (billingModeValue === 'per_request' || billingModeValue === 'per_second') &&
    isSelectableUpstreamValue(fixedPriceValue, 'model_price')
  ) {
    return 'model_price'
  }

  const exprValue = ratioTypes.billing_expr?.upstreams?.[sourceName]
  if (
    ratioType !== 'billing_expr' &&
    exprValue !== null &&
    exprValue !== undefined &&
    exprValue !== 'same'
  ) {
    return 'billing_expr'
  }
  return ratioType
}

export function getVisibleRatioTypesForSource(
  ratioTypes: Partial<Record<RatioType, RatioDifferenceEntry>>,
  sourceName: string,
  filter?: string
): RatioType[] {
  return getOrderedRatioTypes(ratioTypes, filter).filter(
    (ratioType) =>
      getPreferredSyncField(ratioTypes, ratioType, sourceName) === ratioType
  )
}

export function getAlignedRatioTypes(
  ratioTypes: Partial<Record<RatioType, RatioDifferenceEntry>>,
  sourceNames: string[],
  filter?: string
): RatioType[] {
  const ordered = getOrderedRatioTypes(ratioTypes, filter)
  if (sourceNames.length === 0) return ordered

  const visible = new Set<RatioType>()
  sourceNames.forEach((sourceName) => {
    getVisibleRatioTypesForSource(ratioTypes, sourceName, filter).forEach(
      (ratioType) => visible.add(ratioType)
    )
  })

  return ordered.filter((ratioType) => visible.has(ratioType))
}

export function getBillingCategory(
  ratioType: string
): 'price' | 'ratio' | 'tiered' {
  if (ratioType === 'model_price') return 'price'
  if (ratioType === 'billing_mode' || ratioType === 'billing_expr') {
    return 'tiered'
  }
  return 'ratio'
}

export function isSelectableUpstreamValue(
  value: number | string | 'same' | null | undefined,
  ratioType?: RatioType
): boolean {
  if (value === null || value === undefined || value === 'same') return false
  if (!ratioType || !NUMERIC_SYNC_FIELDS.has(ratioType)) return true
  if (typeof value === 'number') return Number.isFinite(value) && value >= 0
  return value.trim() !== '' && isCompletePricingNumber(value.trim())
}

export function getUpstreamDisplayName(sourceName: string): string {
  const synthesizedPresets = [
    { name: OFFICIAL_CHANNEL_NAME, id: OFFICIAL_CHANNEL_ID },
    { name: MODELS_DEV_PRESET_NAME, id: MODELS_DEV_PRESET_ID },
  ]

  for (const preset of synthesizedPresets) {
    if (sourceName === `${preset.name}(${preset.id})`) {
      return preset.name
    }
  }

  return sourceName
}

export function isSelectedResolutionValue(
  resolutions: ResolutionsMap,
  model: string,
  ratioType: RatioType,
  upstreamValue: number | string | 'same' | null | undefined
): boolean {
  if (!isSelectableUpstreamValue(upstreamValue, ratioType)) return false

  const selectedValue = resolutions[model]?.[ratioType]
  if (selectedValue === undefined) return false

  if (NUMERIC_SYNC_FIELDS.has(ratioType)) {
    const selectedNumber = Number(selectedValue)
    const upstreamNumber = Number(upstreamValue)
    return (
      Number.isFinite(selectedNumber) &&
      Number.isFinite(upstreamNumber) &&
      selectedNumber === upstreamNumber
    )
  }

  return selectedValue === upstreamValue
}

export function deleteResolutionField(
  resolutions: ResolutionsMap,
  model: string,
  ratioType: RatioType
): ResolutionsMap {
  return applyResolutionRemovals(resolutions, [{ model, ratioType }])
}

function getDraftModelResolution(
  drafts: Map<string, Record<string, number | string>>,
  resolutions: ResolutionsMap,
  model: string
): Record<string, number | string> {
  const existingDraft = drafts.get(model)
  if (existingDraft) return existingDraft

  const draft = resolutions[model] ? { ...resolutions[model] } : {}
  drafts.set(model, draft)
  return draft
}

function applyResolutionSelectionToDraft(
  drafts: Map<string, Record<string, number | string>>,
  resolutions: ResolutionsMap,
  differences: Record<string, Partial<Record<RatioType, RatioDifferenceEntry>>>,
  selection: ResolutionSelection
) {
  const modelDiffs = differences[selection.model]
  const preferredType = getPreferredSyncField(
    modelDiffs || {},
    selection.ratioType,
    selection.sourceName
  )
  const preferredValue =
    preferredType === selection.ratioType
      ? selection.value
      : (modelDiffs?.[preferredType]?.upstreams?.[selection.sourceName] ??
        selection.value)

  const finalType = preferredType
  const finalValue = preferredValue as number | string
  const category = getBillingCategory(finalType)
  const newModelRes = getDraftModelResolution(
    drafts,
    resolutions,
    selection.model
  )

  Object.keys(newModelRes).forEach((rt) => {
    if (
      category !== 'tiered' &&
      getBillingCategory(rt) !== 'tiered' &&
      getBillingCategory(rt) !== category
    ) {
      delete newModelRes[rt]
    }
  })

  newModelRes[finalType] = finalValue

  if (category === 'price') {
    delete newModelRes['billing_expr']
    const modeVal = modelDiffs?.billing_mode?.upstreams?.[selection.sourceName]
    if (modeVal === 'per_request' || modeVal === 'per_second') {
      newModelRes['billing_mode'] = modeVal
    } else if (
      newModelRes['billing_mode'] !== 'per_request' &&
      newModelRes['billing_mode'] !== 'per_second'
    ) {
      delete newModelRes['billing_mode']
    }
  }

  if (category === 'ratio') {
    delete newModelRes['billing_mode']
    delete newModelRes['billing_expr']
  }

  if (category === 'tiered') {
    Object.keys(newModelRes).forEach((ratioType) => {
      if (getBillingCategory(ratioType) !== 'tiered') {
        delete newModelRes[ratioType]
      }
    })
  }

  if (category === 'tiered' && modelDiffs) {
    const modeVal = modelDiffs.billing_mode?.upstreams?.[selection.sourceName]
    const exprVal = modelDiffs.billing_expr?.upstreams?.[selection.sourceName]
    const isFixedMode = modeVal === 'per_request' || modeVal === 'per_second'
    if (isFixedMode) {
      newModelRes['billing_mode'] = modeVal
      delete newModelRes['billing_expr']
    } else if (
      modeVal !== undefined &&
      modeVal !== null &&
      modeVal !== 'same'
    ) {
      newModelRes['billing_mode'] = modeVal
    } else if (finalType === 'billing_expr') {
      newModelRes['billing_mode'] = 'tiered_expr'
    }
    if (
      !isFixedMode &&
      exprVal !== undefined &&
      exprVal !== null &&
      exprVal !== 'same'
    ) {
      newModelRes['billing_expr'] = exprVal
    }
  }
}

export function resolveResolutionSelection(
  differences: Record<string, Partial<Record<RatioType, RatioDifferenceEntry>>>,
  selection: ResolutionSelection
): ResolvedResolutionSelection {
  const modelDiffs = differences[selection.model]
  const preferredType = getPreferredSyncField(
    modelDiffs || {},
    selection.ratioType,
    selection.sourceName
  )
  const preferredValue =
    preferredType === selection.ratioType
      ? selection.value
      : (modelDiffs?.[preferredType]?.upstreams?.[selection.sourceName] ??
        selection.value)

  return {
    ...selection,
    ratioType: preferredType,
    value: preferredValue as number | string,
  }
}

export function getEffectiveResolutionSelections(
  differences: Record<string, Partial<Record<RatioType, RatioDifferenceEntry>>>,
  selections: ResolutionSelection[]
): ResolvedResolutionSelection[] {
  const effectiveByKey = new Map<string, ResolvedResolutionSelection>()

  selections.forEach((selection) => {
    const resolved = resolveResolutionSelection(differences, selection)
    const category = getBillingCategory(resolved.ratioType)

    if (category !== 'tiered') {
      for (const [key, existing] of effectiveByKey) {
        if (
          existing.model === resolved.model &&
          getBillingCategory(existing.ratioType) !== 'tiered' &&
          getBillingCategory(existing.ratioType) !== category
        ) {
          effectiveByKey.delete(key)
        }
      }
    }

    effectiveByKey.set(`${resolved.model}\u0000${resolved.ratioType}`, resolved)
  })

  return [...effectiveByKey.values()]
}

export function applyResolutionSelections(
  resolutions: ResolutionsMap,
  differences: Record<string, Partial<Record<RatioType, RatioDifferenceEntry>>>,
  selections: ResolutionSelection[]
): ResolutionsMap {
  if (selections.length === 0) return resolutions

  const next = { ...resolutions }
  const drafts = new Map<string, Record<string, number | string>>()

  selections.forEach((selection) => {
    applyResolutionSelectionToDraft(drafts, resolutions, differences, selection)
  })

  drafts.forEach((draft, model) => {
    if (Object.keys(draft).length === 0) {
      delete next[model]
    } else {
      next[model] = draft
    }
  })

  return next
}

export function applyResolutionSelection(
  resolutions: ResolutionsMap,
  differences: Record<string, Partial<Record<RatioType, RatioDifferenceEntry>>>,
  selection: ResolutionSelection
): ResolutionsMap {
  return applyResolutionSelections(resolutions, differences, [selection])
}

export function applyResolutionRemovals(
  resolutions: ResolutionsMap,
  removals: ResolutionRemoval[]
): ResolutionsMap {
  if (removals.length === 0) return resolutions

  const plan: ResolutionRemovalPlan = new Map()
  removals.forEach((removal) => {
    const ratioTypes = plan.get(removal.model)
    if (ratioTypes) {
      ratioTypes.add(removal.ratioType)
    } else {
      plan.set(removal.model, new Set([removal.ratioType]))
    }
  })

  return applyResolutionRemovalPlan(resolutions, plan)
}

export function applyResolutionRemovalPlan(
  resolutions: ResolutionsMap,
  plan: ResolutionRemovalPlan
): ResolutionsMap {
  if (plan.size === 0) return resolutions

  const next = { ...resolutions }

  plan.forEach((ratioTypes, model) => {
    const current = resolutions[model]
    if (!current) return

    const draft = { ...current }
    ratioTypes.forEach((ratioType) => {
      delete draft[ratioType]
      if (
        ratioType === 'model_price' &&
        (draft['billing_mode'] === 'per_request' ||
          draft['billing_mode'] === 'per_second')
      ) {
        delete draft['billing_mode']
      }
      if (ratioType === 'billing_expr') delete draft['billing_mode']
      if (ratioType === 'billing_mode') delete draft['billing_expr']
    })
    if (Object.keys(draft).length === 0) {
      delete next[model]
    } else {
      next[model] = draft
    }
  })

  return next
}

function optionKeyBySyncField(ratioType: string): keyof PricingOptionMaps {
  if (ratioType === 'billing_mode') return 'billing_setting.billing_mode'
  if (ratioType === 'billing_expr') return 'billing_setting.billing_expr'
  return ratioType
    .split('_')
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join('') as keyof PricingOptionMaps
}

export function buildSyncedPricingOptions(
  current: PricingOptionMaps,
  resolutions: ResolutionsMap
): PricingOptionMaps {
  const finalRatios: PricingOptionMaps = {
    ModelRatio: { ...current.ModelRatio },
    CompletionRatio: { ...current.CompletionRatio },
    CacheRatio: { ...current.CacheRatio },
    CreateCacheRatio: { ...current.CreateCacheRatio },
    ImageRatio: { ...current.ImageRatio },
    AudioRatio: { ...current.AudioRatio },
    AudioCompletionRatio: { ...current.AudioCompletionRatio },
    ModelPrice: { ...current.ModelPrice },
    'billing_setting.billing_mode': {
      ...current['billing_setting.billing_mode'],
    },
    'billing_setting.billing_expr': {
      ...current['billing_setting.billing_expr'],
    },
  }

  Object.entries(resolutions).forEach(([model, ratios]) => {
    const selectedTypes = Object.keys(ratios)
    const hasInvalidNumericValue = Object.entries(ratios).some(
      ([ratioType, value]) =>
        NUMERIC_SYNC_FIELDS.has(ratioType) &&
        !isSelectableUpstreamValue(value, ratioType as RatioType)
    )
    if (hasInvalidNumericValue) return

    const hasPrice = selectedTypes.includes('model_price')
    const hasRatio = selectedTypes.some((ratioType) =>
      RATIO_SYNC_FIELDS.includes(ratioType as RatioType)
    )
    const hasTiered =
      selectedTypes.includes('billing_expr') ||
      ratios.billing_mode === 'tiered_expr'
    if (hasTiered && (hasPrice || hasRatio)) return

    if (hasPrice) {
      delete finalRatios.ModelRatio[model]
      delete finalRatios.CompletionRatio[model]
      delete finalRatios.CacheRatio[model]
      delete finalRatios.CreateCacheRatio[model]
      delete finalRatios.ImageRatio[model]
      delete finalRatios.AudioRatio[model]
      delete finalRatios.AudioCompletionRatio[model]
      delete finalRatios['billing_setting.billing_expr'][model]

      const selectedMode = ratios.billing_mode
      const currentMode = finalRatios['billing_setting.billing_mode'][model]
      const selectedFixedMode =
        selectedMode === 'per_request' || selectedMode === 'per_second'
      const currentFixedMode =
        currentMode === 'per_request' || currentMode === 'per_second'
      if (!selectedFixedMode && !currentFixedMode) {
        delete finalRatios['billing_setting.billing_mode'][model]
      }
    }
    if (hasRatio) {
      delete finalRatios.ModelPrice[model]
      delete finalRatios['billing_setting.billing_mode'][model]
      delete finalRatios['billing_setting.billing_expr'][model]
    }

    Object.entries(ratios).forEach(([ratioType, value]) => {
      if (
        (hasRatio &&
          (ratioType === 'model_price' ||
            ratioType === 'billing_mode' ||
            ratioType === 'billing_expr')) ||
        (hasPrice && ratioType === 'billing_expr')
      ) {
        return
      }
      const optionKey = optionKeyBySyncField(ratioType)
      const optionMap = finalRatios[optionKey] as Record<
        string,
        number | string
      >
      optionMap[model] = NUMERIC_SYNC_FIELDS.has(ratioType)
        ? Number(value)
        : value
    })
  })

  return finalRatios
}

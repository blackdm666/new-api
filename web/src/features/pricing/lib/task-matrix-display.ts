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
import type { BillingUsageSchema } from '../types'
import {
  parseTaskTiersFromExpr,
  splitBillingExprAndRequestRules,
  type ParsedTaskTier,
  type TaskTierCondition,
} from './billing-expr'
import { readConditionalTaskPricing } from './billing-expression/task-display'
import {
  getTaskEnumCombinations,
  getTaskEnumFields,
  taskMatrixRowLabel,
  tryParseTaskMatrixConfig,
} from './task-expr'

export type TaskCompactPricingMatrix = {
  rowField: string
  columnField?: string
  rowValues: string[]
  columnValues: string[]
  cells: Map<string, ParsedTaskTier>
}

export function taskCompactMatrixCellKey(
  rowValue: string,
  columnValue: string
) {
  return `${rowValue}\u0000${columnValue}`
}

/**
 * Return a compact view for the common one- or two-enum task pricing shape.
 * The model can opt into this layout without changing the billing expression.
 * Unsupported or ambiguous expressions return null so the caller can retain
 * the existing tier table.
 */
export function getTaskCompactPricingMatrix(
  expression: string | null | undefined,
  schema: BillingUsageSchema | null | undefined
): TaskCompactPricingMatrix | null {
  const enumFields = getTaskEnumFields(schema)
  if (enumFields.length === 0 || enumFields.length > 2) return null

  const tiers = getTaskMatrixDisplayTiers(expression, schema)
  if (!tiers?.length) return null

  const [rowEntry, columnEntry] = enumFields
  const rowValues = rowEntry[1].enum ?? []
  const columnValues = columnEntry?.[1].enum ?? ['']
  if (rowValues.length === 0 || columnValues.length === 0) return null

  const cells = new Map<string, ParsedTaskTier>()
  for (const tier of tiers) {
    const rowValue = tier.conditions.find(
      (condition) => condition.field === rowEntry[0]
    )?.value
    const columnValue = columnEntry
      ? tier.conditions.find((condition) => condition.field === columnEntry[0])
          ?.value
      : ''
    if (!rowValue || (columnEntry && !columnValue)) return null
    cells.set(taskCompactMatrixCellKey(rowValue, columnValue ?? ''), tier)
  }

  if (
    cells.size !== rowValues.length * columnValues.length ||
    rowValues.some((rowValue) =>
      columnValues.some(
        (columnValue) =>
          !cells.has(taskCompactMatrixCellKey(rowValue, columnValue))
      )
    )
  ) {
    return null
  }

  return {
    rowField: rowEntry[0],
    columnField: columnEntry?.[0],
    rowValues,
    columnValues,
    cells,
  }
}

/**
 * Marketplace display helper: expand a recognized task matrix (flat/uniform
 * or a full enum partition) into one row per combination. Returns null when
 * the schema has no enum fields or the expression is not a recognized matrix,
 * so callers keep the raw parsed-tier display.
 */
export function getTaskMatrixDisplayTiers(
  expression: string | null | undefined,
  schema: BillingUsageSchema | null | undefined
): ParsedTaskTier[] | null {
  if (!schema) return null
  const enumFields = getTaskEnumFields(schema)
  if (enumFields.length === 0) return null

  const matrix = tryParseTaskMatrixConfig(expression, schema)
  if (matrix) {
    return matrix.rows.map((row) => ({
      label: taskMatrixRowLabel(row.combination),
      conditions: Object.entries(row.combination)
        .sort(([left], [right]) => left.localeCompare(right))
        .map(([field, value]) => ({ field, value })),
      constant: row.constant,
      unitPrices: { ...row.unitPrices },
    }))
  }

  // The expression may encode the same matrix inside a tier's arithmetic,
  // for example `u("resolution") == "1080p" ? 10 : 5`. The general display
  // parser can evaluate that shape even though the visual matrix parser
  // cannot derive a branch tree from it. Expand its resolved tiers over the
  // declared combinations so the compact table does not require redundant
  // branches for every equal-price resolution.
  const resolvedTiers = getTaskPricingDisplayTiers(expression, schema)
  if (
    resolvedTiers.length === 0 ||
    resolvedTiers.some((tier) => tier.conditionText)
  ) {
    return null
  }

  const enumFieldNames = new Set(enumFields.map(([field]) => field))
  if (
    resolvedTiers.some((tier) =>
      tier.conditions.some((condition) => !enumFieldNames.has(condition.field))
    )
  ) {
    return null
  }

  const combinations = getTaskEnumCombinations(schema)
  const rows = combinations
    .map((combination) => {
      const tier = resolvedTiers.find((candidate) =>
        candidate.conditions.every(
          (condition) => combination[condition.field] === condition.value
        )
      )
      if (!tier) return null

      return {
        label: taskMatrixRowLabel(combination),
        conditions: Object.entries(combination)
          .sort(([left], [right]) => left.localeCompare(right))
          .map(([field, value]) => ({ field, value })),
        constant: tier.constant,
        unitPrices: { ...tier.unitPrices },
      }
    })
    .filter((tier): tier is ParsedTaskTier => tier !== null)
  return rows.length === combinations.length ? rows : null
}

/** Display explicit conditions for a fallback only when its complement is unique.
 * Unlike the editor matrix, unrelated schema fields do not expand the price table.
 */
export function getTaskPricingDisplayTiers(
  expression: string | null | undefined,
  schema: BillingUsageSchema | null | undefined
): ParsedTaskTier[] {
  const tiers = parseTaskTiersFromExpr(expression || '', schema, true)
  if (tiers.length === 0 && expression && schema) {
    const { billingExpr } = splitBillingExprAndRequestRules(expression)
    return readConditionalTaskPricing(billingExpr, schema) ?? []
  }
  const fallback = tiers.at(-1)
  if (!schema || tiers.length < 2 || !fallback) return tiers
  const previous = tiers.slice(0, -1)
  const fields = [
    ...new Set(
      previous.flatMap((tier) =>
        tier.conditions.map((condition) => condition.field)
      )
    ),
  ].sort()
  let combinations: TaskTierCondition[][] = [[]]
  for (const field of fields) {
    const definition = schema[field]
    const values =
      definition?.type === 'boolean' ? ['false', 'true'] : definition?.enum
    // Avoid expanding large plugin schemas merely to name a fallback row.
    if (!values?.length || combinations.length * values.length > 256) {
      return tiers
    }
    combinations = combinations.flatMap((combination) =>
      values.map((value) => [...combination, { field, value }])
    )
  }
  const remaining = combinations.filter(
    (combination) =>
      !previous.some((tier) =>
        tier.conditions.every((condition) =>
          combination.some(
            (value) =>
              value.field === condition.field && value.value === condition.value
          )
        )
      )
  )
  if (remaining.length !== 1) return tiers
  return [...previous, { ...fallback, conditions: remaining[0] }]
}

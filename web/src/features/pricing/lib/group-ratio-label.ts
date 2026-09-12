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
function formatRatioNumber(value: number): string {
  if (Number.isInteger(value)) return value.toString()
  return value.toFixed(6).replace(/0+$/, '').replace(/\.$/, '')
}

function isChineseLanguage(language?: string): boolean {
  return language?.toLocaleLowerCase().startsWith('zh') ?? false
}

function isTraditionalChineseLanguage(language?: string): boolean {
  return (
    language?.toLocaleLowerCase().replaceAll('-', '').startsWith('zhtw') ??
    false
  )
}

export function formatGroupPricingRatio(
  ratio: number,
  language?: string
): string {
  if (isChineseLanguage(language) && ratio >= 0 && ratio < 1) {
    return `${formatRatioNumber(ratio * 10)}折`
  }
  return `${formatRatioNumber(ratio)}x`
}

export function getGroupPricingRatioHeader(
  ratios: number[],
  language: string | undefined,
  ratioLabel: string
): string {
  if (
    isChineseLanguage(language) &&
    ratios.length > 0 &&
    ratios.every((ratio) => ratio >= 0 && ratio < 1)
  ) {
    return isTraditionalChineseLanguage(language) ? '優惠折扣' : '优惠折扣'
  }
  return ratioLabel
}

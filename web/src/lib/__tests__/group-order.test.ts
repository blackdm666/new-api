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
import { describe, expect, it } from 'vitest'

import { movePinnedGroupsLast, orderGroupsPinnedBottom } from '../group-order'

const groups = {
  'AZ-GPT-T6号池': 'azure',
  'AZ-GPT可蒸馏': 'distill',
  'Claude Kiro': 'kiro',
  auto: 'auto',
  生图模型: 'image',
}

describe('orderGroupsPinnedBottom', () => {
  it('moves pinned groups to the end in pinned order and keeps the rest in server order', () => {
    const ordered = orderGroupsPinnedBottom(groups, [
      'AZ-GPT可蒸馏',
      'AZ-GPT-T6号池',
    ])

    expect(Object.keys(ordered)).toEqual([
      'Claude Kiro',
      'auto',
      '生图模型',
      'AZ-GPT可蒸馏',
      'AZ-GPT-T6号池',
    ])
    expect(ordered['AZ-GPT-T6号池']).toBe('azure')
  })

  it('ignores pinned names the user cannot see', () => {
    const ordered = orderGroupsPinnedBottom(groups, ['hidden', 'Claude Kiro'])

    expect(Object.keys(ordered)).toEqual([
      'AZ-GPT-T6号池',
      'AZ-GPT可蒸馏',
      'auto',
      '生图模型',
      'Claude Kiro',
    ])
  })

  it('returns the groups unchanged when nothing is pinned', () => {
    expect(orderGroupsPinnedBottom(groups, undefined)).toBe(groups)
    expect(orderGroupsPinnedBottom(groups, [])).toBe(groups)
  })
})

describe('movePinnedGroupsLast', () => {
  it('lists a model group set with pinned groups last so another group is shown first', () => {
    expect(
      movePinnedGroupsLast(
        ['AZ-GPT-T6号池', 'GPT-Plus', 'AZ-GPT可蒸馏', 'GPT-Pro'],
        ['AZ-GPT-T6号池', 'AZ-GPT可蒸馏']
      )
    ).toEqual(['GPT-Plus', 'GPT-Pro', 'AZ-GPT-T6号池', 'AZ-GPT可蒸馏'])
  })
})

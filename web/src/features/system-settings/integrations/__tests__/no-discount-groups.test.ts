import { describe, expect, it } from 'vitest'

import { parseJsonStringArray } from '../utils'

describe('parseJsonStringArray for no-discount groups', () => {
  it('returns the stored group names trimmed and de-duplicated', () => {
    expect(
      parseJsonStringArray('["代理商", " 特殊代理商 ", "代理商"]')
    ).toEqual(['代理商', '特殊代理商'])
  })

  it('returns an empty list when the option is unset or empty', () => {
    expect(parseJsonStringArray('')).toEqual([])
    expect(parseJsonStringArray('[]')).toEqual([])
  })

  it('ignores invalid JSON, non-array values and non-string entries', () => {
    expect(parseJsonStringArray('{"代理商":true}')).toEqual([])
    expect(parseJsonStringArray('[代理商')).toEqual([])
    expect(parseJsonStringArray('["代理商", 1, null, ""]')).toEqual(['代理商'])
  })
})

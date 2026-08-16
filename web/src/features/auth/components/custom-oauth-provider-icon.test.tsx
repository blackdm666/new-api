/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import assert from 'node:assert/strict'

import { describe, test } from 'vitest'

import { resolveCustomOAuthBrand } from './custom-oauth-provider-brand'

describe('custom OAuth provider branding', () => {
  test('recognizes Google from its configured icon', () => {
    assert.equal(
      resolveCustomOAuthBrand({ name: 'Company login', icon: 'google' }),
      'google'
    )
  })

  test('recognizes Microsoft from its slug or display name', () => {
    assert.equal(
      resolveCustomOAuthBrand({ slug: 'microsoft', name: 'Work account' }),
      'microsoft'
    )
    assert.equal(
      resolveCustomOAuthBrand({ name: 'Continue with Microsoft' }),
      'microsoft'
    )
  })

  test('leaves unrelated providers unchanged', () => {
    assert.equal(
      resolveCustomOAuthBrand({ name: 'Internal SSO', icon: 'openid' }),
      null
    )
  })
})

/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import assert from 'node:assert/strict'

import { describe, test } from 'vitest'

import { oauthProviderGridClassName } from '../oauth-provider-layout'

describe('OAuth provider button layout', () => {
  test('uses one column by default and two columns from the small breakpoint', () => {
    const classes = new Set(oauthProviderGridClassName.split(' '))

    assert.ok(classes.has('grid'))
    assert.ok(classes.has('grid-cols-1'))
    assert.ok(classes.has('sm:grid-cols-2'))
    assert.ok(!classes.has('grid-cols-2'))
  })
})

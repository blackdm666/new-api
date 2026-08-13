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
import assert from 'node:assert/strict'
import { test } from 'node:test'

import type { TFunction } from 'i18next'

import { buildSidebarData } from '../use-sidebar-data'
import { buildTopNavLinks, INFINITE_CANVAS_URL } from '../use-top-nav-links'

const translate = ((key: string) => key) as TFunction

test('places Infinite Canvas immediately after Model Square', () => {
  const links = buildTopNavLinks({
    modules: {
      home: true,
      console: true,
      pricing: { enabled: true, requireAuth: false },
      rankings: { enabled: true, requireAuth: false },
      docs: true,
      about: true,
    },
    isAuthed: true,
    t: translate,
  })

  const pricingIndex = links.findIndex((link) => link.href === '/pricing')
  const canvas = links[pricingIndex + 1]

  assert.deepEqual(canvas, {
    title: 'Infinite Canvas',
    href: INFINITE_CANVAS_URL,
    external: true,
  })
})

test('places Infinite Canvas immediately before Playground', () => {
  const sidebar = buildSidebarData(translate)
  const chatItems = sidebar.navGroups.find(
    (group) => group.id === 'chat'
  )?.items

  assert.ok(chatItems)
  assert.deepEqual(
    {
      title: chatItems[0]?.title,
      url: chatItems[0]?.url,
      external: chatItems[0]?.external,
    },
    {
      title: 'Infinite Canvas',
      url: INFINITE_CANVAS_URL,
      external: true,
    }
  )
  assert.deepEqual(
    {
      title: chatItems[1]?.title,
      url: chatItems[1]?.url,
    },
    {
      title: 'Playground',
      url: '/playground',
    }
  )
})

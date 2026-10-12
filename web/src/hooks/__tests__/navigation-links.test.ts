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

import type { TFunction } from 'i18next'
import { test } from 'vitest'

import { parseHeaderNavModules } from '@/lib/nav-modules'
import { ROLE } from '@/lib/roles'

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
      infiniteCanvas: {
        enabled: true,
        name: 'Infinite Canvas',
        url: INFINITE_CANVAS_URL,
      },
      docs: true,
      business: false,
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

test('uses the backend Infinite Canvas name and URL in both navigation entries', () => {
  const modules = parseHeaderNavModules(
    JSON.stringify({
      infiniteCanvas: {
        name: '创作工作台',
        url: 'https://canvas.example.com/workspace',
      },
    })
  )
  const links = buildTopNavLinks({ modules, isAuthed: true, t: translate })
  const canvas = links.find(
    (link) => link.href === 'https://canvas.example.com/workspace'
  )

  assert.deepEqual(canvas, {
    title: '创作工作台',
    href: 'https://canvas.example.com/workspace',
    external: true,
  })

  const sidebar = buildSidebarData(translate, modules.infiniteCanvas)
  const sidebarCanvas = sidebar.navGroups.find((group) => group.id === 'chat')
    ?.items[0]
  assert.ok(sidebarCanvas && 'url' in sidebarCanvas)
  assert.equal(sidebarCanvas.title, '创作工作台')
  assert.equal(sidebarCanvas.url, 'https://canvas.example.com/workspace')
})

test('hides Infinite Canvas from both navigation entries when disabled', () => {
  const modules = parseHeaderNavModules(
    JSON.stringify({
      infiniteCanvas: {
        enabled: false,
        name: '创作工作台',
        url: 'https://canvas.example.com/workspace',
      },
    })
  )

  const links = buildTopNavLinks({ modules, isAuthed: true, t: translate })
  assert.equal(
    links.some((link) => link.href === 'https://canvas.example.com/workspace'),
    false
  )

  const sidebar = buildSidebarData(translate, modules.infiniteCanvas)
  const chatItems = sidebar.navGroups.find(
    (group) => group.id === 'chat'
  )?.items
  assert.ok(chatItems)
  assert.equal(
    chatItems.some(
      (item) =>
        'url' in item && item.url === 'https://canvas.example.com/workspace'
    ),
    false
  )
  assert.equal(chatItems[0]?.title, 'Playground')
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

// HeaderNavModules as published by the 88API production /api/status (2026-10-12).
const PRODUCTION_HEADER_NAV_MODULES = {
  home: true,
  console: true,
  pricing: { enabled: true, requireAuth: false },
  rankings: { enabled: true, requireAuth: true },
  infiniteCanvas: {
    enabled: true,
    name: 'Infinite Canvas',
    url: 'https://img-pro.88api.ai/',
  },
  docs: true,
  about: false,
}
const PRODUCTION_DOCS_LINK = 'https://88api.ai/zh/docs/'

function findBusinessLink(raw: unknown, language?: string) {
  const links = buildTopNavLinks({
    modules: parseHeaderNavModules(raw),
    docsLink: PRODUCTION_DOCS_LINK,
    isAuthed: false,
    language,
    t: translate,
  })
  return links.find((link) => link.trackId === 'business')
}

test('hides Business when HeaderNavModules has no business switch', () => {
  assert.equal(findBusinessLink(undefined), undefined)
  assert.equal(
    findBusinessLink(JSON.stringify(PRODUCTION_HEADER_NAV_MODULES)),
    undefined
  )
})

test('hides Business when the business switch is off', () => {
  assert.equal(findBusinessLink(JSON.stringify({ business: false })), undefined)
})

test('ignores an object business value because the switch is a boolean', () => {
  assert.equal(
    findBusinessLink(JSON.stringify({ business: { enabled: true } })),
    undefined
  )
})

test('places Business right after Docs as a same-tab website link when enabled', () => {
  const modules = parseHeaderNavModules(
    JSON.stringify({ ...PRODUCTION_HEADER_NAV_MODULES, business: true })
  )
  const links = buildTopNavLinks({
    modules,
    docsLink: PRODUCTION_DOCS_LINK,
    isAuthed: false,
    language: 'zhCN',
    t: translate,
  })

  assert.deepEqual(
    links.map((link) => link.href),
    [
      '/',
      '/dashboard',
      '/pricing',
      'https://img-pro.88api.ai/',
      '/rankings',
      PRODUCTION_DOCS_LINK,
      '/business/',
    ]
  )
  const business = links.at(-1)
  assert.ok(business)
  assert.equal(business.title, 'Business')
  assert.equal(business.external, true)
  assert.equal(business.sameTab, true)
  assert.equal(business.overviewTitle, 'Business overview')
  assert.deepEqual(
    business.items?.map((item) => [item.title, item.href, item.trackId]),
    [
      ['Enterprise', '/business/enterprise/', 'business_enterprise'],
      ['Partner Program', '/business/partner/', 'business_partner'],
    ]
  )
})

test('accepts a string "true" for the business switch', () => {
  assert.equal(
    findBusinessLink(JSON.stringify({ business: 'true' }), 'zhCN')?.href,
    '/business/'
  )
})

test('keeps the Infinite Canvas link unchanged when Business is enabled', () => {
  const modules = parseHeaderNavModules(
    JSON.stringify({ ...PRODUCTION_HEADER_NAV_MODULES, business: true })
  )
  const links = buildTopNavLinks({ modules, isAuthed: true, t: translate })

  assert.deepEqual(
    links.find((link) => link.href === 'https://img-pro.88api.ai/'),
    {
      title: 'Infinite Canvas',
      href: 'https://img-pro.88api.ai/',
      external: true,
    }
  )
})

test('localizes the Business pages for every interface language', () => {
  const cases: Array<[string | undefined, string]> = [
    ['zhCN', ''],
    ['zh-CN', ''],
    ['zhTW', '/zh-tw'],
    ['en', '/en'],
    ['fr', '/en'],
    ['ru', '/en'],
    ['ja', '/en'],
    ['vi', '/en'],
    ['xx-invalid', '/en'],
    [undefined, '/en'],
  ]

  for (const [language, prefix] of cases) {
    const business = findBusinessLink(
      JSON.stringify({ business: true }),
      language
    )
    assert.deepEqual(
      [business?.href, ...(business?.items ?? []).map((item) => item.href)],
      [
        `${prefix}/business/`,
        `${prefix}/business/enterprise/`,
        `${prefix}/business/partner/`,
      ],
      `language ${String(language)}`
    )
  }
})

test('places referral and invoice pages between Wallet and Profile', () => {
  const sidebar = buildSidebarData(translate)
  const personalItems = sidebar.navGroups.find(
    (group) => group.id === 'personal'
  )?.items

  assert.ok(personalItems)
  assert.deepEqual(
    personalItems.map((item) => ('url' in item ? item.url : undefined)),
    ['/wallet', '/referral', '/invoices', '/profile', '/security']
  )
})

test('places invoice management immediately after subscriptions', () => {
  const sidebar = buildSidebarData(translate)
  const adminItems = sidebar.navGroups.find(
    (group) => group.id === 'admin'
  )?.items

  assert.ok(adminItems)
  const subscriptionsIndex = adminItems.findIndex(
    (item) => 'url' in item && item.url === '/subscriptions'
  )
  const invoiceManagement = adminItems[subscriptionsIndex + 1]

  assert.ok(invoiceManagement)
  assert.deepEqual(
    {
      title: invoiceManagement.title,
      url: 'url' in invoiceManagement ? invoiceManagement.url : undefined,
    },
    {
      title: 'Invoice Management',
      url: '/admin-invoices',
    }
  )
})

test('shows email marketing only as a Root administration entry', () => {
  const sidebar = buildSidebarData(translate)
  const adminItems = sidebar.navGroups.find(
    (group) => group.id === 'admin'
  )?.items

  assert.ok(adminItems)
  const marketing = adminItems.find(
    (item) => 'url' in item && item.url === '/admin-marketing'
  )
  assert.ok(marketing)
  assert.equal(marketing.title, 'Email Marketing')
  assert.equal(marketing.requiredRole, ROLE.SUPER_ADMIN)
})

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
import type { TFunction } from 'i18next'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { useStatus } from '@/hooks/use-status'
import { INFINITE_CANVAS_NAME, INFINITE_CANVAS_URL } from '@/lib/external-links'
import {
  DEFAULT_INFINITE_CANVAS_LINK,
  type HeaderNavModules,
  parseHeaderNavModulesFromStatus,
} from '@/lib/nav-modules'
import { useAuthStore } from '@/stores/auth-store'

export { INFINITE_CANVAS_URL }

export type TopNavLink = {
  title: string
  href: string
  disabled?: boolean
  requiresAuth?: boolean
  external?: boolean
}

type BuildTopNavLinksOptions = {
  modules: HeaderNavModules
  docsLink?: string
  isAuthed: boolean
  t: TFunction
}

export function buildTopNavLinks(
  options: BuildTopNavLinksOptions
): TopNavLink[] {
  const links: TopNavLink[] = []

  if (options.modules.home !== false) {
    links.push({ title: options.t('Home'), href: '/' })
  }

  if (options.modules.console !== false) {
    links.push({ title: options.t('Console'), href: '/dashboard' })
  }

  const pricing = options.modules.pricing
  if (pricing && typeof pricing === 'object' && pricing.enabled) {
    const requiresAuth = pricing.requireAuth && !options.isAuthed
    links.push({
      title: options.t('Model Square'),
      href: '/pricing',
      requiresAuth,
    })
  }

  const infiniteCanvas =
    options.modules.infiniteCanvas ?? DEFAULT_INFINITE_CANVAS_LINK
  if (infiniteCanvas.enabled) {
    links.push({
      title:
        infiniteCanvas.name === INFINITE_CANVAS_NAME
          ? options.t(INFINITE_CANVAS_NAME)
          : infiniteCanvas.name,
      href: infiniteCanvas.url,
      external: true,
    })
  }

  const rankings = options.modules.rankings
  if (rankings && typeof rankings === 'object' && rankings.enabled) {
    const requiresAuth = rankings.requireAuth && !options.isAuthed
    links.push({
      title: options.t('Rankings'),
      href: '/rankings',
      requiresAuth,
    })
  }

  if (options.modules.docs !== false) {
    if (options.docsLink) {
      links.push({
        title: options.t('Docs'),
        href: options.docsLink,
        external: true,
      })
    } else {
      links.push({ title: options.t('Docs'), href: '/docs' })
    }
  }

  if (options.modules.about !== false) {
    links.push({ title: options.t('About'), href: '/about' })
  }

  return links
}

/**
 * Generate top navigation links based on HeaderNavModules configuration from backend /api/status
 * Backend format example (stringified JSON):
 * {
 *   home: true,
 *   console: true,
 *   pricing: { enabled: true, requireAuth: false },
 *   rankings: { enabled: true, requireAuth: false },
 *   docs: true,
 *   about: true
 * }
 */
export function useTopNavLinks(): TopNavLink[] {
  const { t } = useTranslation()
  const { status } = useStatus()
  const { auth } = useAuthStore()

  // Parse HeaderNavModules
  const modules = useMemo(() => {
    return parseHeaderNavModulesFromStatus(
      status as Record<string, unknown> | null
    )
  }, [status])

  // Documentation link (may be external)
  const docsLink: string | undefined = status?.docs_link as string | undefined

  const isAuthed = !!auth?.user

  return buildTopNavLinks({ modules, docsLink, isAuthed, t })
}

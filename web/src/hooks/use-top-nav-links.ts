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
import { Building2, Megaphone, Plug } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import type { TopNavLink } from '@/components/layout/types'
import { useStatus } from '@/hooks/use-status'
import {
  BUSINESS_ENTERPRISE_PATH,
  BUSINESS_PARTNER_PATH,
  BUSINESS_PATH,
  BUSINESS_REFERRAL_COMMISSION_RANGE,
  BUSINESS_REFERRAL_PATH,
  getLocalizedSitePath,
  INFINITE_CANVAS_NAME,
  INFINITE_CANVAS_URL,
} from '@/lib/external-links'
import {
  DEFAULT_INFINITE_CANVAS_LINK,
  type HeaderNavModules,
  parseHeaderNavModulesFromStatus,
} from '@/lib/nav-modules'
import { useAuthStore } from '@/stores/auth-store'

export { INFINITE_CANVAS_URL }
export type { TopNavLink }

type BuildTopNavLinksOptions = {
  modules: HeaderNavModules
  docsLink?: string
  isAuthed: boolean
  /** Interface language used to pick the localized website pages */
  language?: string
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

  if (options.modules.business === true) {
    // Static website pages: a full page load in the current tab, never the SPA router.
    // Three blocks (2026-10-14): Enterprise (To B), Reseller Partners (downstream
    // API sites) and Referral Rewards (creators / group owners / influencers).
    links.push({
      title: options.t('Business'),
      href: getLocalizedSitePath(options.language, BUSINESS_PATH),
      external: true,
      sameTab: true,
      trackId: 'business',
      overviewTitle: options.t('Business overview'),
      items: [
        {
          title: options.t('Enterprise'),
          description: options.t(
            'Enterprise contracts · Corporate billing · Business inquiries'
          ),
          href: getLocalizedSitePath(
            options.language,
            BUSINESS_ENTERPRISE_PATH
          ),
          icon: Building2,
          trackId: 'business_enterprise',
        },
        {
          title: options.t('Reseller Partners'),
          description: options.t('Downstream API site integration'),
          href: getLocalizedSitePath(options.language, BUSINESS_PARTNER_PATH),
          icon: Plug,
          trackId: 'business_partner',
        },
        {
          title: options.t('Referral Rewards'),
          description: options.t(
            'Referral commission {{range}} · Creators / group owners / influencers',
            { range: BUSINESS_REFERRAL_COMMISSION_RANGE }
          ),
          href: getLocalizedSitePath(options.language, BUSINESS_REFERRAL_PATH),
          icon: Megaphone,
          trackId: 'business_referral',
        },
      ],
    })
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
 *   business: false,
 *   about: true
 * }
 */
export function useTopNavLinks(): TopNavLink[] {
  const { t, i18n } = useTranslation()
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

  return buildTopNavLinks({
    modules,
    docsLink,
    isAuthed,
    language: i18n.resolvedLanguage || i18n.language,
    t,
  })
}

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
import {
  Activity,
  BadgePercent,
  Box,
  CreditCard,
  FileText,
  FlaskConical,
  Key,
  LayoutDashboard,
  ListTodo,
  MailPlus,
  MessageSquare,
  PanelsTopLeft,
  Radio,
  ReceiptText,
  ServerCog,
  Settings,
  Share2,
  User,
  Users,
  Wallet,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import type { SidebarData } from '@/components/layout/types'
import { INFINITE_CANVAS_URL } from '@/lib/external-links'
import { ROLE } from '@/lib/roles'

export function buildSidebarData(t: TFunction): SidebarData {
  return {
    navGroups: [
      {
        id: 'chat',
        title: t('Chat'),
        items: [
          {
            title: t('Infinite Canvas'),
            url: INFINITE_CANVAS_URL,
            icon: PanelsTopLeft,
            external: true,
          },
          {
            title: t('Playground'),
            url: '/playground',
            icon: FlaskConical,
          },
          {
            title: t('Chat'),
            icon: MessageSquare,
            type: 'chat-presets',
          },
        ],
      },
      {
        id: 'general',
        title: t('General'),
        items: [
          {
            title: t('Overview'),
            url: '/dashboard/overview',
            icon: Activity,
          },
          {
            title: t('Dashboard'),
            url: '/dashboard/models',
            icon: LayoutDashboard,
          },
          {
            title: t('API Keys'),
            url: '/keys',
            icon: Key,
          },
          {
            title: t('Usage Logs'),
            url: '/usage-logs/common',
            icon: FileText,
          },
          {
            title: t('Task Logs'),
            url: '/usage-logs/task',
            activeUrls: ['/usage-logs/drawing'],
            configUrls: ['/usage-logs/drawing', '/usage-logs/task'],
            icon: ListTodo,
          },
        ],
      },
      {
        id: 'personal',
        title: t('Personal'),
        items: [
          {
            title: t('Wallet'),
            url: '/wallet',
            icon: Wallet,
          },
          {
            title: t('Referral Program'),
            url: '/referral',
            icon: Share2,
          },
          {
            title: t('Invoice Requests'),
            url: '/invoices',
            icon: ReceiptText,
          },
          {
            title: t('Profile'),
            url: '/profile',
            icon: User,
          },
        ],
      },
      {
        id: 'admin',
        title: t('Admin'),
        items: [
          {
            title: t('Channels'),
            url: '/channels',
            icon: Radio,
            requiredRole: ROLE.ADMIN,
          },
          {
            title: t('Models'),
            url: '/models/metadata',
            icon: Box,
            requiredRole: ROLE.ADMIN,
          },
          {
            title: t('Users'),
            url: '/users',
            icon: Users,
            requiredRole: ROLE.ADMIN,
          },
          {
            title: t('Redemption Codes'),
            url: '/redemption-codes',
            icon: BadgePercent,
            requiredRole: ROLE.ADMIN,
          },
          {
            title: t('Subscriptions'),
            url: '/subscriptions',
            icon: CreditCard,
            requiredRole: ROLE.ADMIN,
          },
          {
            title: t('Invoice Management'),
            url: '/admin-invoices',
            icon: ReceiptText,
            requiredRole: ROLE.ADMIN,
          },
          {
            title: t('Affiliate Management'),
            url: '/admin-affiliates',
            icon: Share2,
            requiredRole: ROLE.ADMIN,
          },
          {
            title: t('Email Marketing'),
            url: '/admin-marketing',
            icon: MailPlus,
            requiredRole: ROLE.SUPER_ADMIN,
          },
          {
            title: t('System Info'),
            url: '/system-info',
            icon: ServerCog,
            requiredRole: ROLE.SUPER_ADMIN,
          },
          {
            title: t('System Settings'),
            url: '/system-settings/site',
            activeUrls: ['/system-settings'],
            icon: Settings,
            requiredRole: ROLE.SUPER_ADMIN,
          },
        ],
      },
    ],
  }
}

/**
 * Root navigation groups for the application sidebar.
 *
 * These are shown when the URL does not match any nested sidebar view
 * registered in `layout/lib/sidebar-view-registry.ts`.
 */
export function useSidebarData(): SidebarData {
  const { t } = useTranslation()
  return buildSidebarData(t)
}

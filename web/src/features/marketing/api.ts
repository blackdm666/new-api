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
import { api } from '@/lib/api'

import type {
  MarketingAudienceRule,
  MarketingAutomation,
  MarketingCampaign,
  MarketingLocalizedContent,
  MarketingOverview,
  MarketingRecipient,
  MarketingSuppression,
  Paginated,
} from './types'

function data<T>(response: {
  data?: { success?: boolean; data?: T; message?: string }
}): T {
  if (!response.data?.success) {
    throw new Error(response.data?.message || 'Marketing request failed')
  }
  return response.data.data as T
}

export async function fetchMarketingOverview(): Promise<MarketingOverview> {
  return data(await api.get('/api/marketing/overview'))
}

export async function fetchMarketingCampaigns(
  page = 1
): Promise<Paginated<MarketingCampaign>> {
  return data(
    await api.get('/api/marketing/campaigns', {
      params: { p: page, page_size: 20 },
    })
  )
}

export async function createMarketingCampaign(payload: {
  name: string
  audience_rule: MarketingAudienceRule
  localized_content: Record<string, MarketingLocalizedContent>
  scheduled_time?: number
}): Promise<MarketingCampaign> {
  return data(await api.post('/api/marketing/campaigns', payload))
}

export async function updateMarketingCampaign(
  id: number,
  payload: {
    name: string
    audience_rule: MarketingAudienceRule
    localized_content: Record<string, MarketingLocalizedContent>
    scheduled_time?: number
  }
): Promise<void> {
  data(await api.put(`/api/marketing/campaigns/${id}`, payload))
}

export async function previewMarketingAudience(
  audienceRule: MarketingAudienceRule
): Promise<number> {
  const result = data<{ total: number }>(
    await api.post('/api/marketing/campaigns/preview', {
      name: 'preview',
      audience_rule: audienceRule,
      localized_content: {
        'zh-CN': { subject: 'preview', body: 'preview' },
      },
    })
  )
  return result.total
}

export async function scheduleMarketingCampaign(
  id: number,
  scheduledTime = 0
): Promise<void> {
  data(
    await api.post(`/api/marketing/campaigns/${id}/schedule`, {
      scheduled_time: scheduledTime || Math.floor(Date.now() / 1000),
    })
  )
}

export async function transitionMarketingCampaign(
  id: number,
  action: 'pause' | 'resume' | 'cancel' | 'clone'
): Promise<void> {
  data(await api.post(`/api/marketing/campaigns/${id}/${action}`))
}

export async function sendMarketingTest(
  localizedContent: Record<string, MarketingLocalizedContent>,
  language: string
): Promise<void> {
  data(
    await api.post('/api/marketing/test', {
      localized_content: localizedContent,
      language,
    })
  )
}

export async function fetchMarketingAutomations(): Promise<
  MarketingAutomation[]
> {
  return data(await api.get('/api/marketing/automations'))
}

export async function updateMarketingAutomation(
  scene: string,
  payload: {
    enabled: boolean
    apply_existing: boolean
    localized_content: Record<string, MarketingLocalizedContent>
  }
): Promise<void> {
  data(await api.put(`/api/marketing/automations/${scene}`, payload))
}

export async function previewMarketingAutomation(
  scene: string
): Promise<number> {
  const result = data<{ total: number }>(
    await api.get(`/api/marketing/automations/${scene}/preview`)
  )
  return result.total
}

export async function fetchMarketingRecipients(
  campaignId: number,
  page: number
): Promise<Paginated<MarketingRecipient>> {
  return data(
    await api.get(`/api/marketing/campaigns/${campaignId}/recipients`, {
      params: { p: page, page_size: 20 },
    })
  )
}

export async function fetchMarketingSuppressions(
  page = 1
): Promise<Paginated<MarketingSuppression>> {
  return data(
    await api.get('/api/marketing/suppressions', {
      params: { p: page, page_size: 20 },
    })
  )
}

export async function createMarketingSuppression(payload: {
  email: string
  reason: string
}): Promise<void> {
  data(await api.post('/api/marketing/suppressions', payload))
}

export async function deleteMarketingSuppression(id: number): Promise<void> {
  data(await api.delete(`/api/marketing/suppressions/${id}`))
}

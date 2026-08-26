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
import {
  keepPreviousData,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type { TFunction } from 'i18next'
import {
  Ban,
  CirclePause,
  CirclePlay,
  Copy,
  Eye,
  MailPlus,
  Pencil,
  RefreshCw,
  Search,
  Send,
  Settings2,
  Trash2,
} from 'lucide-react'
import { useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { MultiSelect } from '@/components/multi-select'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect } from '@/components/ui/native-select'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import {
  EmailQueueRulesSection,
  EmailQueueSection,
} from '@/features/system-settings/integrations/email-queue-section'
import { useStatus } from '@/hooks/use-status'
import { formatLocalCurrencyAmount } from '@/lib/currency'
import { formatTimestampToDate } from '@/lib/format'
import { getUserFacingErrorMessage } from '@/lib/user-facing-error'

import {
  createMarketingCampaign,
  createMarketingSuppression,
  deleteMarketingSuppression,
  fetchMarketingAutomations,
  fetchMarketingCampaigns,
  fetchLatestMarketingAnnouncement,
  fetchMarketingOverview,
  fetchMarketingRecipients,
  fetchMarketingSuppressions,
  fetchMarketingUserGroups,
  previewMarketingAudience,
  previewMarketingAutomation,
  previewMarketingEmail,
  scheduleMarketingCampaign,
  sendMarketingTest,
  transitionMarketingCampaign,
  updateMarketingAutomation,
  updateMarketingCampaign,
} from './api'
import {
  buildMarketingGroupOptions,
  normalizeMarketingGroups,
} from './lib/marketing-groups'
import type {
  MarketingAudienceRule,
  MarketingAutomation,
  MarketingAutomationTriggerConfig,
  MarketingCampaign,
  MarketingLocalizedContent,
} from './types'

const SCENE_LABELS: Record<string, string> = {
  registration_no_first_call: 'Registration without first API request',
  single_topup_winback: 'Single top-up win-back',
  paid_low_balance: 'Paid user low balance',
  trial_low_balance: 'Trial balance almost depleted',
  inactive_user: 'Long-term inactive user',
  affiliate_program_activation: 'Referral program activation',
  announcement: 'New announcement',
  custom: 'Custom campaign',
}

const ACTIVE_AUTOMATION_SCENES = new Set([
  'registration_no_first_call',
  'single_topup_winback',
  'inactive_user',
  'affiliate_program_activation',
  'announcement',
])

const DEFAULT_AUTOMATION_TRIGGER_CONFIGS: Record<
  string,
  MarketingAutomationTriggerConfig
> = {
  registration_no_first_call: {
    registration_wait_hours: 24,
    max_sends_per_user: 1,
    repeat_interval_days: 2,
  },
  single_topup_winback: {
    match_days: 30,
    max_sends_per_user: 1,
    repeat_interval_days: 30,
  },
  inactive_user: {
    match_days: 30,
    max_sends_per_user: 1,
    repeat_interval_days: 30,
  },
  affiliate_program_activation: {
    active_within_days: 30,
    min_request_count: 10,
    min_topup_count: 1,
    max_sends_per_user: 1,
    repeat_interval_days: 30,
  },
  announcement: { expiry_hours: 48 },
}

const STATUS_CLASS: Record<string, string> = {
  draft: 'border-slate-500/40 text-slate-500',
  scheduled: 'border-blue-500/40 text-blue-500',
  running: 'border-emerald-500/40 text-emerald-500',
  paused: 'border-amber-500/40 text-amber-500',
  completed: 'border-violet-500/40 text-violet-500',
  cancelled: 'border-zinc-500/40 text-zinc-500',
}

const STATUS_LABELS: Record<string, string> = {
  draft: 'Draft',
  scheduled: 'Scheduled',
  running: 'Running',
  paused: 'Paused',
  completed: 'Completed',
  cancelled: 'Cancelled',
}

const MARKETING_LANGUAGES = [
  { value: 'zh-CN', label: '简体中文' },
  { value: 'zh-TW', label: '繁體中文' },
  { value: 'en', label: 'English' },
  { value: 'fr', label: 'Français' },
  { value: 'ja', label: '日本語' },
  { value: 'ru', label: 'Русский' },
  { value: 'vi', label: 'Tiếng Việt' },
] as const

const MARKETING_LANGUAGE_LABELS = Object.fromEntries(
  MARKETING_LANGUAGES.map((item) => [item.value, item.label])
) as Record<string, string>

function recipientStatusLabel(status: string, t: TFunction): string {
  switch (status) {
    case 'pending':
      return t('Pending')
    case 'queued':
      return t('Queued')
    case 'delivered':
      return t('Delivered')
    case 'failed':
      return t('Failed')
    case 'skipped':
      return t('Skipped')
    default:
      return status
  }
}

export function MarketingAdminPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [activeTab, setActiveTab] = useState('campaigns')
  const [campaignOpen, setCampaignOpen] = useState(false)
  const [campaignTarget, setCampaignTarget] =
    useState<MarketingCampaign | null>(null)
  const [campaignPage, setCampaignPage] = useState(1)
  const [automationTarget, setAutomationTarget] =
    useState<MarketingAutomation | null>(null)
  const overviewQuery = useQuery({
    queryKey: ['marketing', 'overview'],
    queryFn: fetchMarketingOverview,
  })
  const campaignsQuery = useQuery({
    queryKey: ['marketing', 'campaigns', campaignPage],
    queryFn: () => fetchMarketingCampaigns(campaignPage),
  })
  const automationsQuery = useQuery({
    queryKey: ['marketing', 'automations'],
    queryFn: fetchMarketingAutomations,
  })
  const refresh = async () => {
    await queryClient.invalidateQueries({ queryKey: ['marketing'] })
  }
  const isEmailOperationsTab =
    activeTab === 'email-queue' || activeTab === 'email-queue-rules'

  return (
    <div className='h-full overflow-y-auto px-4 py-6 sm:px-8'>
      <div className='mx-auto w-full max-w-[1500px] space-y-5'>
        <header className='flex flex-wrap items-start justify-between gap-3'>
          <div>
            <h1 className='text-xl font-semibold tracking-tight'>
              {t('Email Marketing')}
            </h1>
            <p className='text-muted-foreground mt-1 text-sm'>
              {t(
                'Reach the right users with controlled campaigns and measure click-to-top-up conversion.'
              )}
            </p>
          </div>
          {!isEmailOperationsTab ? (
            <div className='flex gap-2'>
              <Button
                type='button'
                variant='outline'
                onClick={() => void refresh()}
              >
                <RefreshCw className='size-4' />
                {t('Refresh')}
              </Button>
              <Button
                type='button'
                onClick={() => {
                  setCampaignTarget(null)
                  setCampaignOpen(true)
                }}
              >
                <MailPlus className='size-4' />
                {t('Create campaign')}
              </Button>
            </div>
          ) : null}
        </header>

        {!isEmailOperationsTab ? (
          <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-4 xl:grid-cols-7'>
            <Stat
              label={t('Campaigns')}
              value={overviewQuery.data?.campaigns ?? 0}
            />
            <Stat
              label={t('Waiting')}
              value={overviewQuery.data?.queued ?? 0}
            />
            <Stat
              label={t('Sent')}
              value={overviewQuery.data?.delivered ?? 0}
            />
            <Stat label={t('Failed')} value={overviewQuery.data?.failed ?? 0} />
            <Stat
              label={t('Clicked')}
              value={overviewQuery.data?.clicked ?? 0}
            />
            <Stat
              label={t('Converted')}
              value={overviewQuery.data?.converted ?? 0}
            />
            <Stat
              label={t('Attributed top-up')}
              value={formatLocalCurrencyAmount(
                (overviewQuery.data?.converted_cents ?? 0) / 100,
                { digitsLarge: 2, digitsSmall: 2 }
              )}
            />
          </div>
        ) : null}

        <Tabs value={activeTab} onValueChange={setActiveTab} className='gap-4'>
          <TabsList className='max-w-full flex-wrap justify-start group-data-horizontal/tabs:h-auto'>
            <TabsTrigger value='campaigns'>{t('Campaigns')}</TabsTrigger>
            <TabsTrigger value='automations'>{t('Automations')}</TabsTrigger>
            <TabsTrigger value='recipients'>{t('Sending records')}</TabsTrigger>
            <TabsTrigger value='suppressions'>
              {t('Suppression list')}
            </TabsTrigger>
            <TabsTrigger value='email-queue'>{t('Email Queue')}</TabsTrigger>
            <TabsTrigger value='email-queue-rules'>
              {t('Email queue rules')}
            </TabsTrigger>
          </TabsList>
          <TabsContent value='campaigns'>
            <CampaignTable
              campaigns={campaignsQuery.data?.items ?? []}
              total={campaignsQuery.data?.total ?? 0}
              page={campaignPage}
              loading={campaignsQuery.isLoading}
              onChanged={refresh}
              onPageChange={setCampaignPage}
              onEdit={(campaign) => {
                setCampaignTarget(campaign)
                setCampaignOpen(true)
              }}
            />
          </TabsContent>
          <TabsContent value='automations'>
            <div className='grid gap-4 lg:grid-cols-2'>
              {(automationsQuery.data ?? [])
                .filter((automation) =>
                  ACTIVE_AUTOMATION_SCENES.has(automation.scene)
                )
                .map((automation) => (
                  <AutomationCard
                    key={automation.scene}
                    automation={automation}
                    onEdit={() => setAutomationTarget(automation)}
                  />
                ))}
            </div>
          </TabsContent>
          <TabsContent value='recipients'>
            <RecipientRecords campaigns={campaignsQuery.data?.items ?? []} />
          </TabsContent>
          <TabsContent value='suppressions'>
            <SuppressionList />
          </TabsContent>
          <TabsContent value='email-queue'>
            <EmailQueueSection />
          </TabsContent>
          <TabsContent value='email-queue-rules'>
            <EmailQueueRulesSection />
          </TabsContent>
        </Tabs>
      </div>

      <CampaignDialog
        key={campaignTarget?.id ?? 'new-campaign'}
        open={campaignOpen}
        campaign={campaignTarget}
        onOpenChange={setCampaignOpen}
        onCreated={refresh}
      />
      <AutomationDialog
        key={automationTarget?.scene ?? 'automation-none'}
        automation={automationTarget}
        onOpenChange={(open) => {
          if (!open) setAutomationTarget(null)
        }}
        onSaved={refresh}
      />
    </div>
  )
}

function CampaignTable(props: {
  campaigns: MarketingCampaign[]
  total: number
  page: number
  loading: boolean
  onChanged: () => Promise<void>
  onPageChange: (page: number) => void
  onEdit: (campaign: MarketingCampaign) => void
}) {
  const { t } = useTranslation()
  const [workingId, setWorkingId] = useState(0)
  const [renderedAt] = useState(() => Math.floor(Date.now() / 1000))
  const act = async (
    campaign: MarketingCampaign,
    action: 'schedule' | 'pause' | 'resume' | 'cancel' | 'clone'
  ) => {
    setWorkingId(campaign.id)
    try {
      if (action === 'schedule') {
        await scheduleMarketingCampaign(campaign.id, campaign.scheduled_time)
      } else await transitionMarketingCampaign(campaign.id, action)
      toast.success(t('Campaign updated'))
      await props.onChanged()
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setWorkingId(0)
    }
  }
  return (
    <div className='space-y-3'>
      <div className='overflow-hidden rounded-xl border'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Campaign')}</TableHead>
              <TableHead>{t('Scene')}</TableHead>
              <TableHead>{t('Status')}</TableHead>
              <TableHead>{t('Recipients')}</TableHead>
              <TableHead>{t('Sent')}</TableHead>
              <TableHead>{t('Clicked')}</TableHead>
              <TableHead>{t('Converted')}</TableHead>
              <TableHead>{t('Attributed top-up')}</TableHead>
              <TableHead className='text-right'>{t('Actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {props.campaigns.map((campaign) => (
              <TableRow key={campaign.id}>
                <TableCell>
                  <div className='font-medium'>{campaign.name}</div>
                  <div className='text-muted-foreground text-xs'>
                    #{campaign.id} ·{' '}
                    {formatTimestampToDate(campaign.created_time)}
                  </div>
                  {campaign.paused_reason ? (
                    <div className='text-destructive mt-1 max-w-72 text-xs'>
                      {campaign.paused_reason}
                    </div>
                  ) : null}
                </TableCell>
                <TableCell>
                  {t(SCENE_LABELS[campaign.scene] || campaign.scene)}
                </TableCell>
                <TableCell>
                  <Badge
                    variant='outline'
                    className={STATUS_CLASS[campaign.status]}
                  >
                    {t(STATUS_LABELS[campaign.status] || campaign.status)}
                  </Badge>
                  {campaign.status === 'scheduled' &&
                  campaign.scheduled_time ? (
                    <div className='text-muted-foreground mt-1 text-xs'>
                      {formatTimestampToDate(campaign.scheduled_time)}
                    </div>
                  ) : null}
                </TableCell>
                <TableCell>{campaign.recipient_count}</TableCell>
                <TableCell>{campaign.delivered_count}</TableCell>
                <TableCell>{campaign.clicked_count}</TableCell>
                <TableCell>{campaign.converted_count}</TableCell>
                <TableCell>
                  {formatLocalCurrencyAmount(campaign.converted_cents / 100, {
                    digitsLarge: 2,
                    digitsSmall: 2,
                  })}
                </TableCell>
                <TableCell>
                  <div className='flex justify-end gap-1'>
                    {campaign.status === 'draft' ? (
                      <>
                        <Button
                          size='icon-sm'
                          variant='outline'
                          title={t('Edit')}
                          disabled={workingId > 0}
                          onClick={() => props.onEdit(campaign)}
                        >
                          <Pencil className='size-4' />
                        </Button>
                        <Button
                          size='icon-sm'
                          variant='outline'
                          title={
                            campaign.scheduled_time > renderedAt
                              ? t('Schedule')
                              : t('Send now')
                          }
                          disabled={workingId > 0}
                          onClick={() => void act(campaign, 'schedule')}
                        >
                          <Send className='size-4' />
                        </Button>
                      </>
                    ) : null}
                    {campaign.status === 'running' ||
                    campaign.status === 'scheduled' ? (
                      <Button
                        size='icon-sm'
                        variant='outline'
                        title={t('Pause')}
                        disabled={workingId > 0}
                        onClick={() => void act(campaign, 'pause')}
                      >
                        <CirclePause className='size-4' />
                      </Button>
                    ) : null}
                    {campaign.status === 'paused' ? (
                      <Button
                        size='icon-sm'
                        variant='outline'
                        title={t('Resume')}
                        disabled={workingId > 0}
                        onClick={() => void act(campaign, 'resume')}
                      >
                        <CirclePlay className='size-4' />
                      </Button>
                    ) : null}
                    <Button
                      size='icon-sm'
                      variant='outline'
                      title={t('Clone')}
                      disabled={workingId > 0}
                      onClick={() => void act(campaign, 'clone')}
                    >
                      <Copy className='size-4' />
                    </Button>
                    {['draft', 'scheduled', 'running', 'paused'].includes(
                      campaign.status
                    ) ? (
                      <Button
                        size='icon-sm'
                        variant='outline'
                        title={t('Cancel')}
                        disabled={workingId > 0}
                        onClick={() => void act(campaign, 'cancel')}
                      >
                        <Ban className='size-4' />
                      </Button>
                    ) : null}
                  </div>
                </TableCell>
              </TableRow>
            ))}
            {!props.loading && props.campaigns.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={9}
                  className='text-muted-foreground h-28 text-center'
                >
                  {t('No marketing campaigns')}
                </TableCell>
              </TableRow>
            ) : null}
          </TableBody>
        </Table>
      </div>
      <PaginationControls
        page={props.page}
        total={props.total}
        onPageChange={props.onPageChange}
      />
    </div>
  )
}

function AutomationCard(props: {
  automation: MarketingAutomation
  onEdit: () => void
}) {
  const { t } = useTranslation()
  const [preview, setPreview] = useState<number | null>(null)
  const [loading, setLoading] = useState(false)
  const triggerConfig = parseAutomationTriggerConfig(
    props.automation.scene,
    props.automation.trigger_config
  )
  const loadPreview = async () => {
    setLoading(true)
    try {
      setPreview(
        await previewMarketingAutomation(props.automation.scene, triggerConfig)
      )
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setLoading(false)
    }
  }
  return (
    <Card data-card-hover='false'>
      <CardContent className='space-y-4 py-5'>
        <div className='flex items-start justify-between gap-3'>
          <div>
            <h3 className='font-semibold'>
              {t(
                SCENE_LABELS[props.automation.scene] || props.automation.scene
              )}
            </h3>
            <p className='text-muted-foreground mt-1 text-sm'>
              {t(automationDescription(props.automation.scene))}
            </p>
          </div>
          <Badge variant={props.automation.enabled ? 'default' : 'outline'}>
            {props.automation.enabled ? t('Enabled') : t('Disabled')}
          </Badge>
        </div>
        <div className='bg-muted/40 flex items-center justify-between rounded-lg border px-3 py-2'>
          <span className='text-muted-foreground text-sm'>
            {preview === null
              ? t('Audience not calculated')
              : t('{{count}} matching users', { count: preview })}
          </span>
          <Button
            variant='ghost'
            size='sm'
            disabled={loading}
            onClick={() => void loadPreview()}
          >
            <Search className='size-4' />
            {t('Preview audience')}
          </Button>
        </div>
        <Button
          type='button'
          variant='outline'
          className='w-full'
          onClick={props.onEdit}
        >
          <Settings2 className='size-4' />
          {t('Configure automation')}
        </Button>
      </CardContent>
    </Card>
  )
}

function CampaignDialog(props: {
  open: boolean
  campaign: MarketingCampaign | null
  onOpenChange: (open: boolean) => void
  onCreated: () => Promise<void>
}) {
  const { t } = useTranslation()
  const { status } = useStatus()
  const quotaPerUnit = status?.quota_per_unit ?? 500_000
  const initialRule = parseAudienceRule(props.campaign?.audience_rule)
  const initialContent = parseLocalizedContent(
    props.campaign?.localized_content
  )
  const [name, setName] = useState(props.campaign?.name ?? '')
  const [language, setLanguage] = useState('zh-CN')
  const [contents, setContents] = useState(initialContent)
  const [groups, setGroups] = useState(
    normalizeMarketingGroups(initialRule.groups)
  )
  const [inactiveDays, setInactiveDays] = useState(
    valueOrEmpty(initialRule.inactive_days)
  )
  const [topUpMin, setTopUpMin] = useState(
    valueOrEmpty(initialRule.topup_count_min)
  )
  const [topUpMax, setTopUpMax] = useState(
    valueOrEmpty(initialRule.topup_count_max)
  )
  const [lastTopUpBefore, setLastTopUpBefore] = useState(
    timestampToDateInput(initialRule.last_topup_before ?? 0)
  )
  const [quotaMin, setQuotaMin] = useState(
    initialRule.quota_min === undefined
      ? ''
      : String(initialRule.quota_min / quotaPerUnit)
  )
  const [quotaMax, setQuotaMax] = useState(
    initialRule.quota_max === undefined
      ? ''
      : String(initialRule.quota_max / quotaPerUnit)
  )
  const [scheduledAt, setScheduledAt] = useState(
    timestampToLocalInput(props.campaign?.scheduled_time ?? 0)
  )
  const [preview, setPreview] = useState<number | null>(null)
  const [saving, setSaving] = useState(false)
  const groupsQuery = useQuery({
    queryKey: ['marketing', 'user-groups'],
    queryFn: fetchMarketingUserGroups,
    enabled: props.open,
  })
  const groupOptions = useMemo(
    () => buildMarketingGroupOptions(groupsQuery.data ?? [], groups),
    [groupsQuery.data, groups]
  )
  const audienceRule = useMemo<MarketingAudienceRule>(() => {
    const rule: MarketingAudienceRule = {}
    if (groups.length > 0) rule.groups = groups
    if (Number(inactiveDays) > 0) rule.inactive_days = Number(inactiveDays)
    if (topUpMin !== '') rule.topup_count_min = Number(topUpMin)
    if (topUpMax !== '') rule.topup_count_max = Number(topUpMax)
    if (lastTopUpBefore !== '') {
      rule.last_topup_before = dateInputToTimestamp(lastTopUpBefore)
    }
    if (quotaMin !== '') {
      rule.quota_min = Math.round(Number(quotaMin) * quotaPerUnit)
    }
    if (quotaMax !== '') {
      rule.quota_max = Math.round(Number(quotaMax) * quotaPerUnit)
    }
    return rule
  }, [
    groups,
    inactiveDays,
    lastTopUpBefore,
    quotaMax,
    quotaMin,
    quotaPerUnit,
    topUpMax,
    topUpMin,
  ])
  const content = contents[language] ?? { subject: '', body: '' }
  const localizedContent = useMemo(() => {
    return Object.fromEntries(
      Object.entries(contents).filter(
        ([, item]) => item.subject.trim() && item.body.trim()
      )
    )
  }, [contents])
  const fallbackContent = contents['zh-CN'] ?? { subject: '', body: '' }
  const valid =
    name.trim().length > 0 &&
    fallbackContent.subject.trim().length > 0 &&
    fallbackContent.subject.length <= 120 &&
    fallbackContent.body.trim().length > 0 &&
    fallbackContent.body.length <= 5000

  const previewAudience = async () => {
    try {
      setPreview(await previewMarketingAudience(audienceRule))
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    }
  }
  const test = async () => {
    if (!valid) return
    try {
      await sendMarketingTest(localizedContent, language)
      toast.success(t('Test email queued'))
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    }
  }
  const save = async () => {
    if (!valid) return
    setSaving(true)
    try {
      const payload = {
        name: name.trim(),
        audience_rule: audienceRule,
        localized_content: localizedContent,
        scheduled_time: localInputToTimestamp(scheduledAt),
      }
      if (props.campaign) {
        await updateMarketingCampaign(props.campaign.id, payload)
        toast.success(t('Campaign updated'))
      } else {
        await createMarketingCampaign(payload)
        toast.success(t('Campaign draft created'))
      }
      props.onOpenChange(false)
      await props.onCreated()
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='max-h-[90vh] overflow-y-auto sm:max-w-3xl'>
        <DialogHeader>
          <DialogTitle>
            {props.campaign
              ? t('Edit email campaign')
              : t('Create email campaign')}
          </DialogTitle>
          <DialogDescription>
            {t(
              'Only the subject and content are editable. Brand layout and links stay protected.'
            )}
          </DialogDescription>
        </DialogHeader>
        <div className='grid gap-5 md:grid-cols-2'>
          <div className='space-y-4'>
            <Field label={t('Campaign name')}>
              <Input
                value={name}
                maxLength={160}
                onChange={(event) => setName(event.target.value)}
              />
            </Field>
            <NativeSelect
              value={language}
              onChange={(event) => setLanguage(event.target.value)}
              aria-label={t('Template language')}
            >
              {MARKETING_LANGUAGES.map((item) => (
                <option key={item.value} value={item.value}>
                  {item.label}
                </option>
              ))}
            </NativeSelect>
            <Field
              label={t('Email subject')}
              hint={`${content.subject.length}/120`}
            >
              <Input
                value={content.subject}
                maxLength={120}
                onChange={(event) =>
                  setContents((current) => ({
                    ...current,
                    [language]: { ...content, subject: event.target.value },
                  }))
                }
              />
            </Field>
            <Field
              label={t('Email content')}
              hint={`${content.body.length}/5000`}
            >
              <Textarea
                value={content.body}
                maxLength={5000}
                rows={10}
                onChange={(event) =>
                  setContents((current) => ({
                    ...current,
                    [language]: { ...content, body: event.target.value },
                  }))
                }
              />
            </Field>
          </div>
          <div className='space-y-4'>
            <Field
              label={t('User groups')}
              hint={
                groupsQuery.isError
                  ? t('Failed to load user groups')
                  : t('Options stay synchronized with current system groups')
              }
            >
              <MultiSelect
                id='marketing-user-groups'
                options={groupOptions}
                selected={groups}
                onChange={setGroups}
                placeholder={
                  groupsQuery.isLoading
                    ? t('Loading...')
                    : t('Select user groups...')
                }
                disabled={groupsQuery.isLoading}
                maxVisibleChips={3}
              />
            </Field>
            <Field label={t('Inactive days')}>
              <Input
                type='number'
                min={0}
                value={inactiveDays}
                onChange={(event) => setInactiveDays(event.target.value)}
              />
            </Field>
            <div className='grid grid-cols-2 gap-3'>
              <Field label={t('Minimum top-ups')}>
                <Input
                  type='number'
                  min={0}
                  value={topUpMin}
                  onChange={(event) => setTopUpMin(event.target.value)}
                />
              </Field>
              <Field label={t('Maximum top-ups')}>
                <Input
                  type='number'
                  min={0}
                  value={topUpMax}
                  onChange={(event) => setTopUpMax(event.target.value)}
                />
              </Field>
            </div>
            <Field label={t('Last top-up before')}>
              <Input
                type='date'
                value={lastTopUpBefore}
                onChange={(event) => setLastTopUpBefore(event.target.value)}
              />
            </Field>
            <div className='grid grid-cols-2 gap-3'>
              <Field label={t('Minimum displayed balance')}>
                <Input
                  type='number'
                  min={0}
                  step='0.1'
                  value={quotaMin}
                  onChange={(event) => setQuotaMin(event.target.value)}
                />
              </Field>
              <Field label={t('Maximum displayed balance')}>
                <Input
                  type='number'
                  min={0}
                  step='0.1'
                  value={quotaMax}
                  onChange={(event) => setQuotaMax(event.target.value)}
                />
              </Field>
            </div>
            <Field label={t('Scheduled send time')}>
              <Input
                type='datetime-local'
                value={scheduledAt}
                onChange={(event) => setScheduledAt(event.target.value)}
              />
            </Field>
            <Card data-card-hover='false'>
              <CardContent className='flex items-center justify-between py-4'>
                <div>
                  <div className='text-sm font-medium'>
                    {t('Audience preview')}
                  </div>
                  <div className='text-muted-foreground text-xs'>
                    {preview === null
                      ? t('Not calculated')
                      : t('{{count}} matching users', { count: preview })}
                  </div>
                </div>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={() => void previewAudience()}
                >
                  <Search className='size-4' />
                  {t('Calculate')}
                </Button>
              </CardContent>
            </Card>
          </div>
        </div>
        <DialogFooter>
          <Button
            type='button'
            variant='outline'
            disabled={!valid}
            onClick={() => void test()}
          >
            {t('Send test')}
          </Button>
          <Button
            type='button'
            disabled={!valid || saving}
            onClick={() => void save()}
          >
            {props.campaign ? t('Save changes') : t('Save draft')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function AutomationDialog(props: {
  automation: MarketingAutomation | null
  onOpenChange: (open: boolean) => void
  onSaved: () => Promise<void>
}) {
  const { t } = useTranslation()
  const initial = parseLocalizedContent(props.automation?.localized_content)
  const initialTriggerConfig = parseAutomationTriggerConfig(
    props.automation?.scene ?? '',
    props.automation?.trigger_config
  )
  const [language, setLanguage] = useState('zh-CN')
  const [enabled, setEnabled] = useState(props.automation?.enabled ?? false)
  const [applyExisting, setApplyExisting] = useState(
    props.automation?.apply_existing ?? false
  )
  const [contents, setContents] =
    useState<Record<string, MarketingLocalizedContent>>(initial)
  const [triggerConfig, setTriggerConfig] =
    useState<MarketingAutomationTriggerConfig>(initialTriggerConfig)
  const [saving, setSaving] = useState(false)
  const [insertingAnnouncement, setInsertingAnnouncement] = useState(false)
  const [previewing, setPreviewing] = useState(false)
  const [previewOpen, setPreviewOpen] = useState(false)
  const [previewSubject, setPreviewSubject] = useState('')
  const [previewBody, setPreviewBody] = useState('')
  const content = contents[language] ?? { subject: '', body: '' }
  const automationScene = props.automation?.scene ?? ''
  const triggerConfigValid = validAutomationTriggerConfig(
    automationScene,
    triggerConfig
  )
  const previewQuery = useQuery({
    queryKey: [
      'marketing',
      'automation-preview',
      automationScene,
      triggerConfig,
    ],
    queryFn: () => previewMarketingAutomation(automationScene, triggerConfig),
    enabled: automationScene !== '' && triggerConfigValid,
  })
  const fallbackContent = contents['zh-CN'] ?? { subject: '', body: '' }
  const valid =
    triggerConfigValid &&
    fallbackContent.subject.trim().length > 0 &&
    fallbackContent.subject.length <= 120 &&
    fallbackContent.body.trim().length > 0 &&
    fallbackContent.body.length <= 5000 &&
    Object.values(contents).every(
      (item) =>
        item.subject.trim().length > 0 &&
        item.subject.length <= 120 &&
        item.body.trim().length > 0 &&
        item.body.length <= 5000
    )
  const insertLatestAnnouncement = async () => {
    setInsertingAnnouncement(true)
    try {
      const announcement = await fetchLatestMarketingAnnouncement()
      if (!announcement) {
        toast.error(t('No published announcement available'))
        return
      }
      const announcementText = [announcement.content, announcement.extra]
        .map((item) => item.trim())
        .filter(Boolean)
        .join('\n\n')
      if (announcementText.length > 5000) {
        toast.error(
          t('Latest announcement does not fit in the email content limit')
        )
        return
      }
      setContents((current) => ({
        ...current,
        [language]: { ...content, body: announcementText },
      }))
      toast.success(t('Latest announcement inserted'))
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setInsertingAnnouncement(false)
    }
  }
  const preview = async () => {
    if (!valid) return
    setPreviewing(true)
    try {
      const rendered = await previewMarketingEmail(
        contents,
        language,
        automationScene
      )
      setPreviewSubject(rendered.subject)
      setPreviewBody(rendered.body)
      setPreviewOpen(true)
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setPreviewing(false)
    }
  }
  const save = async () => {
    if (!props.automation || !valid) return
    setSaving(true)
    try {
      await updateMarketingAutomation(props.automation.scene, {
        enabled,
        apply_existing: applyExisting,
        trigger_config: triggerConfig,
        localized_content: contents,
      })
      toast.success(t('Automation updated'))
      props.onOpenChange(false)
      await props.onSaved()
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    } finally {
      setSaving(false)
    }
  }
  return (
    <Dialog open={props.automation !== null} onOpenChange={props.onOpenChange}>
      <DialogContent className='max-h-[90vh] overflow-y-auto sm:max-w-2xl'>
        <DialogHeader>
          <DialogTitle>
            {props.automation
              ? t(
                  SCENE_LABELS[props.automation.scene] || props.automation.scene
                )
              : t('Automation')}
          </DialogTitle>
          <DialogDescription>
            {t(
              'The email layout and destination are fixed. Configure only localized copy and activation.'
            )}
          </DialogDescription>
        </DialogHeader>
        <div className='flex items-center justify-between rounded-xl border p-4'>
          <div>
            <Label>{t('Enable automation')}</Label>
            <p className='text-muted-foreground mt-1 text-xs'>
              {t('New matching users will be evaluated by the scheduler.')}
            </p>
          </div>
          <Switch checked={enabled} onCheckedChange={setEnabled} />
        </div>
        {enabled ? (
          <label className='flex items-start gap-3 rounded-xl border p-4'>
            <Checkbox
              checked={applyExisting}
              onCheckedChange={(checked) => setApplyExisting(Boolean(checked))}
            />
            <span>
              <span className='block text-sm font-medium'>
                {automationScene === 'announcement'
                  ? t(
                      'Send the latest published announcement to existing users'
                    )
                  : t('Process users who already match')}
              </span>
              <span className='text-muted-foreground block text-xs'>
                {automationScene === 'announcement'
                  ? t(
                      'Only the latest published announcement is backfilled; older announcements are never sent.'
                    )
                  : t(
                      'If disabled, only users matching after this automation is enabled are included.'
                    )}
              </span>
              <span className='text-muted-foreground mt-1 block text-xs'>
                {previewQuery.isLoading
                  ? t('Loading...')
                  : t('{{count}} users currently match', {
                      count: previewQuery.data ?? 0,
                    })}
              </span>
            </span>
          </label>
        ) : null}
        {automationScene === 'registration_no_first_call' ? (
          <div className='grid gap-3 sm:grid-cols-3'>
            <Field label={t('Wait after registration (hours)')}>
              <Input
                type='number'
                min={1}
                max={8760}
                aria-label={t('Wait after registration (hours)')}
                value={triggerConfig.registration_wait_hours ?? 24}
                onChange={(event) =>
                  setTriggerConfig((current) => ({
                    ...current,
                    registration_wait_hours: Number(event.target.value),
                  }))
                }
              />
            </Field>
            <Field label={t('Maximum sends per user')}>
              <Input
                type='number'
                min={1}
                max={10}
                aria-label={t('Maximum sends per user')}
                value={triggerConfig.max_sends_per_user ?? 1}
                onChange={(event) =>
                  setTriggerConfig((current) => ({
                    ...current,
                    max_sends_per_user: Number(event.target.value),
                  }))
                }
              />
            </Field>
            <Field label={t('Repeat interval (days)')}>
              <Input
                type='number'
                min={1}
                max={3650}
                disabled={(triggerConfig.max_sends_per_user ?? 1) <= 1}
                aria-label={t('Repeat interval (days)')}
                value={triggerConfig.repeat_interval_days ?? 2}
                onChange={(event) =>
                  setTriggerConfig((current) => ({
                    ...current,
                    repeat_interval_days: Number(event.target.value),
                  }))
                }
              />
            </Field>
          </div>
        ) : null}
        {automationScene === 'single_topup_winback' ||
        automationScene === 'inactive_user' ? (
          <div className='grid gap-3 sm:grid-cols-3'>
            <Field
              label={
                automationScene === 'single_topup_winback'
                  ? t('No repeat top-up for (days)')
                  : t('Inactive for (days)')
              }
            >
              <Input
                type='number'
                min={1}
                max={3650}
                aria-label={
                  automationScene === 'single_topup_winback'
                    ? t('No repeat top-up for (days)')
                    : t('Inactive for (days)')
                }
                value={triggerConfig.match_days ?? 30}
                onChange={(event) =>
                  setTriggerConfig((current) => ({
                    ...current,
                    match_days: Number(event.target.value),
                  }))
                }
              />
            </Field>
            <Field label={t('Maximum sends per user')}>
              <Input
                type='number'
                min={1}
                max={10}
                aria-label={t('Maximum sends per user')}
                value={triggerConfig.max_sends_per_user ?? 1}
                onChange={(event) =>
                  setTriggerConfig((current) => ({
                    ...current,
                    max_sends_per_user: Number(event.target.value),
                  }))
                }
              />
            </Field>
            <Field label={t('Repeat interval (days)')}>
              <Input
                type='number'
                min={1}
                max={3650}
                disabled={(triggerConfig.max_sends_per_user ?? 1) <= 1}
                aria-label={t('Repeat interval (days)')}
                value={triggerConfig.repeat_interval_days ?? 30}
                onChange={(event) =>
                  setTriggerConfig((current) => ({
                    ...current,
                    repeat_interval_days: Number(event.target.value),
                  }))
                }
              />
            </Field>
          </div>
        ) : null}
        {automationScene === 'affiliate_program_activation' ? (
          <div className='space-y-2'>
            <div className='grid gap-3 sm:grid-cols-3'>
              <Field label={t('Active API use within (days)')}>
                <Input
                  type='number'
                  min={1}
                  max={3650}
                  aria-label={t('Active API use within (days)')}
                  value={triggerConfig.active_within_days ?? 30}
                  onChange={(event) =>
                    setTriggerConfig((current) => ({
                      ...current,
                      active_within_days: Number(event.target.value),
                    }))
                  }
                />
              </Field>
              <Field label={t('Minimum successful API requests')}>
                <Input
                  type='number'
                  min={1}
                  max={1_000_000_000}
                  aria-label={t('Minimum successful API requests')}
                  value={triggerConfig.min_request_count ?? 10}
                  onChange={(event) =>
                    setTriggerConfig((current) => ({
                      ...current,
                      min_request_count: Number(event.target.value),
                    }))
                  }
                />
              </Field>
              <Field label={t('Minimum eligible top-ups')}>
                <Input
                  type='number'
                  min={1}
                  max={1000}
                  aria-label={t('Minimum eligible top-ups')}
                  value={triggerConfig.min_topup_count ?? 1}
                  onChange={(event) =>
                    setTriggerConfig((current) => ({
                      ...current,
                      min_topup_count: Number(event.target.value),
                    }))
                  }
                />
              </Field>
              <Field label={t('Maximum sends per user')}>
                <Input
                  type='number'
                  min={1}
                  max={10}
                  aria-label={t('Maximum sends per user')}
                  value={triggerConfig.max_sends_per_user ?? 1}
                  onChange={(event) =>
                    setTriggerConfig((current) => ({
                      ...current,
                      max_sends_per_user: Number(event.target.value),
                    }))
                  }
                />
              </Field>
              <Field label={t('Repeat interval (days)')}>
                <Input
                  type='number'
                  min={1}
                  max={3650}
                  disabled={(triggerConfig.max_sends_per_user ?? 1) <= 1}
                  aria-label={t('Repeat interval (days)')}
                  value={triggerConfig.repeat_interval_days ?? 30}
                  onChange={(event) =>
                    setTriggerConfig((current) => ({
                      ...current,
                      repeat_interval_days: Number(event.target.value),
                    }))
                  }
                />
              </Field>
            </div>
            <p className='text-muted-foreground text-xs'>
              {t(
                'This automation only runs while the referral commission program is enabled.'
              )}
            </p>
          </div>
        ) : null}
        {automationScene === 'announcement' ? (
          <Field label={t('Announcement email validity (hours)')}>
            <Input
              type='number'
              min={1}
              max={168}
              aria-label={t('Announcement email validity (hours)')}
              value={triggerConfig.expiry_hours ?? 48}
              onChange={(event) =>
                setTriggerConfig((current) => ({
                  ...current,
                  expiry_hours: Number(event.target.value),
                }))
              }
            />
          </Field>
        ) : null}
        <Field label={t('Current editing language')}>
          <div className='space-y-1.5'>
            <NativeSelect
              value={language}
              onChange={(event) => setLanguage(event.target.value)}
              aria-label={t('Current editing language')}
            >
              {MARKETING_LANGUAGES.map((item) => (
                <option key={item.value} value={item.value}>
                  {item.label}
                </option>
              ))}
            </NativeSelect>
            <p className='text-muted-foreground text-xs'>
              {t(
                "Emails follow each recipient's language. If a template is missing, Simplified Chinese is used."
              )}
            </p>
          </div>
        </Field>
        <Field
          label={t('Email subject')}
          hint={`${content.subject.length}/120`}
        >
          <Input
            value={content.subject}
            maxLength={120}
            onChange={(event) =>
              setContents((current) => ({
                ...current,
                [language]: { ...content, subject: event.target.value },
              }))
            }
          />
        </Field>
        <Field
          label={t('Email content')}
          hint={`${content.body.length}/5000`}
          action={
            automationScene === 'announcement' ? (
              <Button
                type='button'
                variant='outline'
                size='sm'
                disabled={insertingAnnouncement}
                onClick={() => void insertLatestAnnouncement()}
              >
                <Copy className='size-3.5' />
                {t('Insert latest announcement')}
              </Button>
            ) : undefined
          }
        >
          <Textarea
            value={content.body}
            maxLength={5000}
            rows={7}
            onChange={(event) =>
              setContents((current) => ({
                ...current,
                [language]: { ...content, body: event.target.value },
              }))
            }
          />
        </Field>
        <DialogFooter>
          <Button
            type='button'
            variant='outline'
            disabled={previewing || !valid}
            onClick={() => void preview()}
          >
            <Eye className='size-4' />
            {t('Preview email')}
          </Button>
          <Button
            type='button'
            disabled={saving || !valid}
            onClick={() => void save()}
          >
            {t('Save automation')}
          </Button>
        </DialogFooter>
      </DialogContent>
      <Dialog open={previewOpen} onOpenChange={setPreviewOpen}>
        <DialogContent className='sm:max-w-4xl'>
          <DialogHeader>
            <DialogTitle>{t('Email preview')}</DialogTitle>
          </DialogHeader>
          <div className='space-y-3'>
            <div className='space-y-1'>
              <Label className='text-muted-foreground text-xs font-normal'>
                {t('Subject')}
              </Label>
              <div className='bg-muted rounded-md px-3 py-2 text-sm'>
                {previewSubject || '-'}
              </div>
            </div>
            <div className='space-y-1'>
              <Label className='text-muted-foreground text-xs font-normal'>
                {t('Body preview')}
              </Label>
              <iframe
                title='marketing-email-preview'
                srcDoc={previewBody}
                sandbox=''
                className='h-[600px] w-full rounded-md border bg-white'
              />
            </div>
          </div>
          <DialogFooter>
            <DialogClose render={<Button variant='outline' />}>
              {t('Close')}
            </DialogClose>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Dialog>
  )
}

function RecipientRecords(props: { campaigns: MarketingCampaign[] }) {
  const { t } = useTranslation()
  const [campaignId, setCampaignId] = useState(props.campaigns[0]?.id ?? 0)
  const [engagement, setEngagement] = useState('')
  const [page, setPage] = useState(1)
  const recipientsQuery = useQuery({
    queryKey: ['marketing', 'recipients', campaignId, engagement, page],
    queryFn: () => fetchMarketingRecipients(campaignId, page, engagement),
    enabled: campaignId > 0,
    placeholderData: keepPreviousData,
  })
  return (
    <div className='space-y-4'>
      <div className='flex flex-wrap gap-3'>
        <Select
          items={[
            { value: '0', label: t('Select campaign') },
            ...props.campaigns.map((campaign) => ({
              value: String(campaign.id),
              label: `#${campaign.id} ${campaign.name}`,
            })),
          ]}
          value={String(campaignId)}
          onValueChange={(value) => {
            setCampaignId(Number(value))
            setPage(1)
          }}
        >
          <SelectTrigger className='w-64' aria-label={t('Campaign')}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            <SelectGroup>
              <SelectItem value='0'>{t('Select campaign')}</SelectItem>
              {props.campaigns.map((campaign) => (
                <SelectItem key={campaign.id} value={String(campaign.id)}>
                  #{campaign.id} {campaign.name}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
        <Select
          items={[
            { value: 'all', label: t('All records') },
            { value: 'clicked', label: t('Clicked recipients') },
            { value: 'converted', label: t('Converted recipients') },
          ]}
          value={engagement || 'all'}
          onValueChange={(value) => {
            setEngagement(value && value !== 'all' ? value : '')
            setPage(1)
          }}
        >
          <SelectTrigger className='w-44' aria-label={t('Interaction status')}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            <SelectGroup>
              <SelectItem value='all'>{t('All records')}</SelectItem>
              <SelectItem value='clicked'>{t('Clicked recipients')}</SelectItem>
              <SelectItem value='converted'>
                {t('Converted recipients')}
              </SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
      </div>
      <div className='overflow-hidden rounded-xl border'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('User')}</TableHead>
              <TableHead>{t('Recipient')}</TableHead>
              <TableHead>{t('Language')}</TableHead>
              <TableHead>{t('Status')}</TableHead>
              <TableHead>{t('Sent at')}</TableHead>
              <TableHead>{t('Clicked at')}</TableHead>
              <TableHead>{t('Converted at')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {(recipientsQuery.data?.items ?? []).map((item) => (
              <TableRow key={item.id}>
                <TableCell>{item.username || '-'}</TableCell>
                <TableCell>{item.recipient_masked}</TableCell>
                <TableCell>
                  {MARKETING_LANGUAGE_LABELS[item.language] || item.language}
                </TableCell>
                <TableCell>{recipientStatusLabel(item.status, t)}</TableCell>
                <TableCell>
                  {item.delivered_time
                    ? formatTimestampToDate(item.delivered_time)
                    : '-'}
                </TableCell>
                <TableCell>
                  {item.clicked_time
                    ? formatTimestampToDate(item.clicked_time)
                    : '-'}
                </TableCell>
                <TableCell>
                  {item.converted_time
                    ? formatTimestampToDate(item.converted_time)
                    : '-'}
                </TableCell>
              </TableRow>
            ))}
            {campaignId > 0 &&
            !recipientsQuery.isLoading &&
            (recipientsQuery.data?.items.length ?? 0) === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={7}
                  className='text-muted-foreground h-24 text-center'
                >
                  {t('No sending records')}
                </TableCell>
              </TableRow>
            ) : null}
          </TableBody>
        </Table>
      </div>
      <PaginationControls
        page={page}
        total={recipientsQuery.data?.total ?? 0}
        onPageChange={setPage}
      />
    </div>
  )
}

function SuppressionList() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [email, setEmail] = useState('')
  const [reason, setReason] = useState('')
  const [page, setPage] = useState(1)
  const query = useQuery({
    queryKey: ['marketing', 'suppressions', page],
    queryFn: () => fetchMarketingSuppressions(page),
  })
  const add = async () => {
    try {
      await createMarketingSuppression({
        email: email.trim(),
        reason: reason.trim(),
      })
      setEmail('')
      setReason('')
      await queryClient.invalidateQueries({
        queryKey: ['marketing', 'suppressions'],
      })
      toast.success(t('Suppression added'))
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    }
  }
  const remove = async (id: number) => {
    try {
      await deleteMarketingSuppression(id)
      await queryClient.invalidateQueries({
        queryKey: ['marketing', 'suppressions'],
      })
    } catch (error) {
      toast.error(getUserFacingErrorMessage(error))
    }
  }
  return (
    <div className='space-y-4'>
      <Card data-card-hover='false'>
        <CardContent className='grid gap-3 py-4 md:grid-cols-[1fr_1fr_auto]'>
          <Input
            value={email}
            placeholder={t('Email address')}
            onChange={(event) => setEmail(event.target.value)}
          />
          <Input
            value={reason}
            placeholder={t('Suppression reason')}
            onChange={(event) => setReason(event.target.value)}
          />
          <Button
            type='button'
            disabled={!email.trim()}
            onClick={() => void add()}
          >
            <Ban className='size-4' />
            {t('Add suppression')}
          </Button>
        </CardContent>
      </Card>
      <div className='overflow-hidden rounded-xl border'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Recipient')}</TableHead>
              <TableHead>{t('Reason')}</TableHead>
              <TableHead>{t('Created at')}</TableHead>
              <TableHead className='text-right'>{t('Actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {(query.data?.items ?? []).map((item) => (
              <TableRow key={item.id}>
                <TableCell>{item.email_masked}</TableCell>
                <TableCell>{item.reason}</TableCell>
                <TableCell>
                  {formatTimestampToDate(item.created_time)}
                </TableCell>
                <TableCell className='text-right'>
                  <Button
                    type='button'
                    size='icon-sm'
                    variant='outline'
                    onClick={() => void remove(item.id)}
                  >
                    <Trash2 className='size-4' />
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
      <PaginationControls
        page={page}
        total={query.data?.total ?? 0}
        onPageChange={setPage}
      />
    </div>
  )
}

function PaginationControls(props: {
  page: number
  total: number
  onPageChange: (page: number) => void
}) {
  const { t } = useTranslation()
  const totalPages = Math.max(1, Math.ceil(props.total / 20))
  return (
    <div className='flex items-center justify-between gap-3'>
      <span className='text-muted-foreground text-sm'>
        {t('{{count}} records', { count: props.total })}
      </span>
      <div className='flex gap-2'>
        <Button
          type='button'
          size='sm'
          variant='outline'
          disabled={props.page <= 1}
          onClick={() => props.onPageChange(props.page - 1)}
        >
          {t('Previous')}
        </Button>
        <Button
          type='button'
          size='sm'
          variant='outline'
          disabled={props.page >= totalPages}
          onClick={() => props.onPageChange(props.page + 1)}
        >
          {t('Next')}
        </Button>
      </div>
    </div>
  )
}

function Field(props: {
  label: string
  hint?: string
  action?: ReactNode
  children: ReactNode
}) {
  return (
    <div className='space-y-1.5'>
      <div className='flex items-center justify-between gap-3'>
        <Label>{props.label}</Label>
        {props.action || props.hint ? (
          <div className='flex items-center gap-2'>
            {props.action}
            {props.hint ? (
              <span className='text-muted-foreground text-xs'>
                {props.hint}
              </span>
            ) : null}
          </div>
        ) : null}
      </div>
      {props.children}
    </div>
  )
}

function Stat(props: { label: string; value: number | string }) {
  return (
    <Card data-card-hover='false'>
      <CardContent className='py-4'>
        <div className='text-muted-foreground text-xs'>{props.label}</div>
        <div className='mt-1 text-xl font-semibold'>{props.value}</div>
      </CardContent>
    </Card>
  )
}

function parseLocalizedContent(
  raw: string | undefined
): Record<string, MarketingLocalizedContent> {
  try {
    const parsed = JSON.parse(raw || '{}') as Record<
      string,
      MarketingLocalizedContent
    >
    return {
      ...parsed,
      'zh-CN': parsed['zh-CN'] ?? { subject: '', body: '' },
      en: parsed.en ?? { subject: '', body: '' },
    }
  } catch {
    return {
      'zh-CN': { subject: '', body: '' },
      en: { subject: '', body: '' },
    }
  }
}

function parseAudienceRule(raw: string | undefined): MarketingAudienceRule {
  try {
    return JSON.parse(raw || '{}') as MarketingAudienceRule
  } catch {
    return {}
  }
}

function valueOrEmpty(value: number | undefined) {
  return value === undefined ? '' : String(value)
}

function timestampToLocalInput(timestamp: number) {
  if (!timestamp) return ''
  const date = new Date(timestamp * 1000)
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000)
  return local.toISOString().slice(0, 16)
}

function timestampToDateInput(timestamp: number) {
  if (!timestamp) return ''
  const date = new Date(timestamp * 1000)
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000)
  return local.toISOString().slice(0, 10)
}

function localInputToTimestamp(value: string) {
  if (!value) return 0
  const timestamp = new Date(value).getTime()
  return Number.isFinite(timestamp) ? Math.floor(timestamp / 1000) : 0
}

function dateInputToTimestamp(value: string) {
  const timestamp = new Date(`${value}T23:59:59`).getTime()
  return Number.isFinite(timestamp) ? Math.floor(timestamp / 1000) : 0
}

function parseAutomationTriggerConfig(
  scene: string,
  raw: string | undefined
): MarketingAutomationTriggerConfig {
  const defaults = DEFAULT_AUTOMATION_TRIGGER_CONFIGS[scene] ?? {}
  try {
    const parsed = JSON.parse(raw || '{}') as MarketingAutomationTriggerConfig
    return { ...defaults, ...parsed }
  } catch {
    return { ...defaults }
  }
}

function validAutomationTriggerConfig(
  scene: string,
  config: MarketingAutomationTriggerConfig
) {
  if (scene === 'registration_no_first_call') {
    return (
      Number(config.registration_wait_hours) >= 1 &&
      Number(config.registration_wait_hours) <= 8760 &&
      Number(config.max_sends_per_user) >= 1 &&
      Number(config.max_sends_per_user) <= 10 &&
      Number(config.repeat_interval_days) >= 1 &&
      Number(config.repeat_interval_days) <= 3650
    )
  }
  if (scene === 'single_topup_winback' || scene === 'inactive_user') {
    return (
      Number(config.match_days) >= 1 &&
      Number(config.match_days) <= 3650 &&
      Number(config.max_sends_per_user) >= 1 &&
      Number(config.max_sends_per_user) <= 10 &&
      Number(config.repeat_interval_days) >= 1 &&
      Number(config.repeat_interval_days) <= 3650
    )
  }
  if (scene === 'affiliate_program_activation') {
    return (
      Number(config.active_within_days) >= 1 &&
      Number(config.active_within_days) <= 3650 &&
      Number(config.min_request_count) >= 1 &&
      Number(config.min_request_count) <= 1_000_000_000 &&
      Number(config.min_topup_count) >= 1 &&
      Number(config.min_topup_count) <= 1000 &&
      Number(config.max_sends_per_user) >= 1 &&
      Number(config.max_sends_per_user) <= 10 &&
      Number(config.repeat_interval_days) >= 1 &&
      Number(config.repeat_interval_days) <= 3650
    )
  }
  if (scene === 'announcement') {
    return (
      Number(config.expiry_hours) >= 1 && Number(config.expiry_hours) <= 168
    )
  }
  return false
}

function automationDescription(scene: string): string {
  const descriptions: Record<string, string> = {
    registration_no_first_call:
      'Users who registered long enough ago but have not completed their first successful API request.',
    single_topup_winback:
      'Users with exactly one eligible wallet top-up and no repeat top-up for the configured period.',
    inactive_user: 'Users who have not signed in for the configured period.',
    affiliate_program_activation:
      'Users with eligible top-ups who meet the configured usage activity requirements.',
    announcement: 'Enabled users receive each published announcement once.',
  }
  return descriptions[scene] || scene
}

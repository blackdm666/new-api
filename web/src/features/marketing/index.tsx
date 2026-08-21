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
import {
  Ban,
  CirclePause,
  CirclePlay,
  Copy,
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
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect } from '@/components/ui/native-select'
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
  fetchMarketingOverview,
  fetchMarketingRecipients,
  fetchMarketingSuppressions,
  fetchMarketingUserGroups,
  previewMarketingAudience,
  previewMarketingAutomation,
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
  MarketingCampaign,
  MarketingLocalizedContent,
} from './types'

const SCENE_LABELS: Record<string, string> = {
  single_topup_winback: 'Single top-up win-back',
  paid_low_balance: 'Paid user low balance',
  trial_low_balance: 'Trial balance almost depleted',
  inactive_user: 'Long-term inactive user',
  announcement: 'New announcement',
  custom: 'Custom campaign',
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

export function MarketingAdminPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
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
        </header>

        <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-4 xl:grid-cols-7'>
          <Stat
            label={t('Campaigns')}
            value={overviewQuery.data?.campaigns ?? 0}
          />
          <Stat label={t('Waiting')} value={overviewQuery.data?.queued ?? 0} />
          <Stat label={t('Sent')} value={overviewQuery.data?.delivered ?? 0} />
          <Stat label={t('Failed')} value={overviewQuery.data?.failed ?? 0} />
          <Stat label={t('Clicked')} value={overviewQuery.data?.clicked ?? 0} />
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

        <Tabs defaultValue='campaigns' className='gap-4'>
          <TabsList className='max-w-full flex-wrap justify-start group-data-horizontal/tabs:h-auto'>
            <TabsTrigger value='campaigns'>{t('Campaigns')}</TabsTrigger>
            <TabsTrigger value='automations'>{t('Automations')}</TabsTrigger>
            <TabsTrigger value='recipients'>{t('Sending records')}</TabsTrigger>
            <TabsTrigger value='suppressions'>
              {t('Suppression list')}
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
              {(automationsQuery.data ?? []).map((automation) => (
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
                            campaign.scheduled_time >
                            Math.floor(Date.now() / 1000)
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
  const loadPreview = async () => {
    setLoading(true)
    try {
      setPreview(await previewMarketingAutomation(props.automation.scene))
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
  const [language, setLanguage] = useState('zh-CN')
  const [enabled, setEnabled] = useState(props.automation?.enabled ?? false)
  const [applyExisting, setApplyExisting] = useState(
    props.automation?.apply_existing ?? false
  )
  const [contents, setContents] =
    useState<Record<string, MarketingLocalizedContent>>(initial)
  const [saving, setSaving] = useState(false)
  const content = contents[language] ?? { subject: '', body: '' }
  const automationScene = props.automation?.scene ?? ''
  const previewQuery = useQuery({
    queryKey: ['marketing', 'automation-preview', automationScene],
    queryFn: () => previewMarketingAutomation(automationScene),
    enabled: automationScene !== '',
  })
  const fallbackContent = contents['zh-CN'] ?? { subject: '', body: '' }
  const valid =
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
  const save = async () => {
    if (!props.automation || !valid) return
    setSaving(true)
    try {
      await updateMarketingAutomation(props.automation.scene, {
        enabled,
        apply_existing: applyExisting,
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
      <DialogContent className='sm:max-w-2xl'>
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
                {t('Process users who already match')}
              </span>
              <span className='text-muted-foreground block text-xs'>
                {t(
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
        <NativeSelect
          value={language}
          onChange={(event) => setLanguage(event.target.value)}
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
        <Field label={t('Email content')} hint={`${content.body.length}/5000`}>
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
            disabled={saving || !valid}
            onClick={() => void save()}
          >
            {t('Save automation')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function RecipientRecords(props: { campaigns: MarketingCampaign[] }) {
  const { t } = useTranslation()
  const [campaignId, setCampaignId] = useState(props.campaigns[0]?.id ?? 0)
  const [page, setPage] = useState(1)
  const recipientsQuery = useQuery({
    queryKey: ['marketing', 'recipients', campaignId, page],
    queryFn: () => fetchMarketingRecipients(campaignId, page),
    enabled: campaignId > 0,
    placeholderData: keepPreviousData,
  })
  return (
    <div className='space-y-4'>
      <NativeSelect
        value={campaignId}
        onChange={(event) => {
          setCampaignId(Number(event.target.value))
          setPage(1)
        }}
        aria-label={t('Campaign')}
      >
        <option value={0}>{t('Select campaign')}</option>
        {props.campaigns.map((campaign) => (
          <option key={campaign.id} value={campaign.id}>
            #{campaign.id} {campaign.name}
          </option>
        ))}
      </NativeSelect>
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
                <TableCell>{item.language}</TableCell>
                <TableCell>{t(item.status)}</TableCell>
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

function Field(props: { label: string; hint?: string; children: ReactNode }) {
  return (
    <div className='space-y-1.5'>
      <div className='flex items-center justify-between gap-3'>
        <Label>{props.label}</Label>
        {props.hint ? (
          <span className='text-muted-foreground text-xs'>{props.hint}</span>
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

function automationDescription(scene: string): string {
  const descriptions: Record<string, string> = {
    single_topup_winback:
      'Users with exactly one eligible wallet top-up and no repeat top-up for 30 days.',
    paid_low_balance:
      'Paying users whose displayed balance falls to 1.0 or below.',
    trial_low_balance:
      'Users who consumed trial quota, never paid, and have 0.1 balance or less.',
    inactive_user: 'Users who have not signed in for at least 30 days.',
    announcement: 'Enabled users receive each published announcement once.',
  }
  return descriptions[scene] || scene
}

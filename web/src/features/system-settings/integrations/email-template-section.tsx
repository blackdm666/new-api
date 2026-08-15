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
  BellRing,
  ChevronDown,
  Code2,
  Eye,
  Gauge,
  HandCoins,
  LockKeyhole,
  MailCheck,
  ReceiptText,
  RadioTower,
  RotateCcw,
  Save,
  ScanSearch,
  ShieldCheck,
  type LucideIcon,
} from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import { Textarea } from '@/components/ui/textarea'
import { api } from '@/lib/api'
import { cn } from '@/lib/utils'

import { SettingsPageActionsPortal } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { EmailDeliveryFailures } from './email-delivery-failures'

type TemplateVariable = {
  name: string
  description?: string
}

type EmailTemplate = {
  key: string
  name: string
  description?: string
  default_subject: string
  default_body: string
  current_subject: string
  current_body: string
  customized: boolean
  variables: TemplateVariable[]
}

type TemplateGroup = 'account' | 'operations' | 'invoice' | 'affiliate'
type TemplateAudience = 'user' | 'administrator' | 'notification'

type TemplateMeta = {
  name: string
  description: string
  group: TemplateGroup
  audience: TemplateAudience
  icon: LucideIcon
}

const TEMPLATE_LANGUAGES = [
  { value: 'zh-CN', label: '简体中文' },
  { value: 'zh-TW', label: '繁體中文' },
  { value: 'en', label: 'English' },
  { value: 'fr', label: 'Français' },
  { value: 'ja', label: '日本語' },
  { value: 'ru', label: 'Русский' },
  { value: 'vi', label: 'Tiếng Việt' },
] as const

const TEMPLATE_GROUPS: Array<{
  key: TemplateGroup
  label: string
}> = [
  { key: 'account', label: 'Account and security' },
  { key: 'operations', label: 'Operations alerts' },
  { key: 'invoice', label: 'Invoice Management' },
  { key: 'affiliate', label: 'Referral Program' },
]

const TEMPLATE_META: Record<string, TemplateMeta> = {
  account_verification_user: {
    name: 'Registration and email verification',
    description:
      'Verification code sent when a user registers or verifies an email address.',
    group: 'account',
    audience: 'user',
    icon: MailCheck,
  },
  password_reset_user: {
    name: 'Password reset',
    description: 'Secure reset link sent after a user requests a new password.',
    group: 'account',
    audience: 'user',
    icon: LockKeyhole,
  },
  quota_warning_user: {
    name: 'Quota warning',
    description: 'Sent when a user quota falls below the warning threshold.',
    group: 'operations',
    audience: 'user',
    icon: Gauge,
  },
  channel_status_admin: {
    name: 'Channel status notification',
    description:
      'Sent to administrators when a channel is disabled, restored, or changes status.',
    group: 'operations',
    audience: 'administrator',
    icon: RadioTower,
  },
  inspection_alert_admin: {
    name: 'Inspection alert',
    description:
      'Sent to administrators after channel tests or upstream model inspections report results.',
    group: 'operations',
    audience: 'administrator',
    icon: ScanSearch,
  },
  invoice_request_admin: {
    name: 'New invoice application notification',
    description:
      'Sent to invoice administrators after a user submits an invoice application.',
    group: 'invoice',
    audience: 'administrator',
    icon: ReceiptText,
  },
  invoice_issued_user: {
    name: 'Issued invoice user notification',
    description: 'Sent to the applicant after the invoice has been issued.',
    group: 'invoice',
    audience: 'user',
    icon: ReceiptText,
  },
  invoice_expiry_admin: {
    name: 'Expiring invoice application reminder',
    description:
      'Sent to invoice administrators 24 hours before a pending application expires.',
    group: 'invoice',
    audience: 'administrator',
    icon: BellRing,
  },
  affiliate_upgrade_admin: {
    name: 'Promoter upgrade eligibility notification',
    description:
      'Sent to the system notification email when a promoter reaches an upgrade criterion.',
    group: 'affiliate',
    audience: 'administrator',
    icon: HandCoins,
  },
  affiliate_upgrade_user: {
    name: 'Promoter tier upgrade notification',
    description:
      'Sent to the promoter after an administrator approves the tier upgrade.',
    group: 'affiliate',
    audience: 'user',
    icon: HandCoins,
  },
  affiliate_commission_user: {
    name: 'Commission review result notification',
    description:
      'Sent to the promoter after a commission is approved or rejected.',
    group: 'affiliate',
    audience: 'user',
    icon: HandCoins,
  },
  affiliate_payout_user: {
    name: 'Commission payout status notification',
    description:
      'Sent to the promoter after a payout is approved, rejected, or paid.',
    group: 'affiliate',
    audience: 'user',
    icon: HandCoins,
  },
}

const AUDIENCE_LABELS: Record<TemplateAudience, string> = {
  user: 'User email',
  administrator: 'Administrator email',
  notification: 'Configured notification recipient',
}

export function EmailTemplateSettingsSection() {
  const { t } = useTranslation()

  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [previewing, setPreviewing] = useState(false)
  const [templates, setTemplates] = useState<EmailTemplate[]>([])
  const [activeKey, setActiveKey] = useState('')
  const [templateLang, setTemplateLang] = useState('zh-CN')
  const [drafts, setDrafts] = useState<
    Record<string, { subject: string; body: string }>
  >({})
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const [previewOpen, setPreviewOpen] = useState(false)
  const [resetOpen, setResetOpen] = useState(false)
  const [previewSubject, setPreviewSubject] = useState('')
  const [previewBody, setPreviewBody] = useState('')
  const [inlinePreviewLoading, setInlinePreviewLoading] = useState(false)

  const bodyRef = useRef<HTMLTextAreaElement>(null)

  const activeTemplate = useMemo(
    () => templates.find((template) => template.key === activeKey),
    [templates, activeKey]
  )
  const currentDraft = drafts[activeKey] || { subject: '', body: '' }
  const hasUnsavedChanges = Boolean(
    activeTemplate &&
    (currentDraft.subject !== activeTemplate.current_subject ||
      currentDraft.body !== activeTemplate.current_body)
  )

  const fetchTemplates = async () => {
    setLoading(true)
    try {
      const response = await api.get(
        `/api/option/email_templates?lang=${encodeURIComponent(templateLang)}`
      )
      const { success, message, data } = response.data
      if (!success) {
        toast.error(message || t('Failed to load email templates'))
        return
      }
      const list: EmailTemplate[] = data || []
      const initial: Record<string, { subject: string; body: string }> = {}
      list.forEach((template) => {
        initial[template.key] = {
          subject: template.current_subject || '',
          body: template.current_body || '',
        }
      })
      setTemplates(list)
      setDrafts(initial)
      setActiveKey((previous) =>
        list.some((template) => template.key === previous)
          ? previous
          : list[0]?.key || ''
      )
    } catch {
      toast.error(t('Failed to load email templates'))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void fetchTemplates()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [templateLang])

  useEffect(() => {
    setAdvancedOpen(false)
  }, [activeKey])

  useEffect(() => {
    if (!activeKey) return
    let cancelled = false
    const timer = window.setTimeout(async () => {
      setInlinePreviewLoading(true)
      try {
        const response = await api.post('/api/option/email_templates/preview', {
          key: activeKey,
          lang: templateLang,
          subject: currentDraft.subject,
          body: currentDraft.body,
        })
        if (!cancelled && response.data?.success) {
          setPreviewSubject(response.data.data?.subject || '')
          setPreviewBody(response.data.data?.body || '')
        }
      } catch {
        if (!cancelled) {
          setPreviewSubject('')
          setPreviewBody('')
        }
      } finally {
        if (!cancelled) setInlinePreviewLoading(false)
      }
    }, 350)
    return () => {
      cancelled = true
      window.clearTimeout(timer)
    }
  }, [activeKey, currentDraft.body, currentDraft.subject, templateLang])

  const updateDraft = (patch: { subject?: string; body?: string }) => {
    setDrafts((previous) => ({
      ...previous,
      [activeKey]: { ...previous[activeKey], ...patch } as {
        subject: string
        body: string
      },
    }))
  }

  const insertVariable = (name: string) => {
    const token = `{{${name}}}`
    const element = bodyRef.current
    if (element && typeof element.selectionStart === 'number') {
      const start = element.selectionStart ?? 0
      const end = element.selectionEnd ?? start
      const current = currentDraft.body || ''
      const next = current.slice(0, start) + token + current.slice(end)
      updateDraft({ body: next })
      requestAnimationFrame(() => {
        element.focus()
        const position = start + token.length
        element.setSelectionRange(position, position)
      })
      return
    }
    updateDraft({ body: (currentDraft.body || '') + token })
  }

  const handlePreview = async () => {
    if (!activeKey) return
    setPreviewing(true)
    try {
      const response = await api.post('/api/option/email_templates/preview', {
        key: activeKey,
        lang: templateLang,
        subject: currentDraft.subject,
        body: currentDraft.body,
      })
      const { success, message, data } = response.data
      if (!success) {
        toast.error(message || t('Preview failed'))
        return
      }
      setPreviewSubject(data?.subject || '')
      setPreviewBody(data?.body || '')
      setPreviewOpen(true)
    } catch {
      toast.error(t('Preview failed'))
    } finally {
      setPreviewing(false)
    }
  }

  const handleSave = async () => {
    if (!activeTemplate) return
    setSaving(true)
    try {
      const response = await api.post('/api/option/email_templates/save', {
        key: activeKey,
        lang: templateLang,
        subject: currentDraft.subject || '',
        body: currentDraft.body || '',
      })
      if (!response.data.success) {
        toast.error(response.data.message)
        return
      }
      toast.success(t('Saved'))
      await fetchTemplates()
    } catch {
      toast.error(t('Save failed'))
    } finally {
      setSaving(false)
    }
  }

  const handleReset = async () => {
    if (!activeTemplate) return
    try {
      const response = await api.post('/api/option/email_templates/reset', {
        key: activeKey,
        lang: templateLang,
      })
      if (!response.data.success) {
        toast.error(response.data.message)
        return
      }
      toast.success(t('Restored to defaults'))
      setResetOpen(false)
      await fetchTemplates()
    } catch {
      toast.error(t('Operation failed'))
    }
  }

  return (
    <SettingsSection title={t('Email Templates')}>
      {activeTemplate ? (
        <SettingsPageActionsPortal>
          <Select
            value={templateLang}
            onValueChange={(value) => value && setTemplateLang(value)}
          >
            <SelectTrigger
              className='h-8 w-36'
              aria-label={t('Template language')}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent
              side='bottom'
              align='end'
              alignItemWithTrigger={false}
            >
              {TEMPLATE_LANGUAGES.map((language) => (
                <SelectItem key={language.value} value={language.value}>
                  {language.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button
            type='button'
            size='sm'
            variant='outline'
            onClick={handlePreview}
            disabled={previewing || saving}
          >
            <Eye data-icon='inline-start' />
            <span>{t('Enlarge preview')}</span>
          </Button>
          <Button
            type='button'
            size='sm'
            variant='outline'
            onClick={() => setResetOpen(true)}
            disabled={saving}
          >
            <RotateCcw data-icon='inline-start' />
            <span>{t('Restore defaults')}</span>
          </Button>
          <Button
            type='button'
            size='sm'
            onClick={handleSave}
            disabled={saving || previewing || !hasUnsavedChanges}
          >
            <Save data-icon='inline-start' />
            <span>{t(saving ? 'Saving...' : 'Save template')}</span>
          </Button>
        </SettingsPageActionsPortal>
      ) : null}

      <p className='text-muted-foreground max-w-3xl text-sm'>
        {t(
          'Manage account, operations, invoice, and referral emails by business purpose. HTML is available only when advanced customization is needed.'
        )}
      </p>

      {loading && templates.length === 0 ? (
        <div className='text-muted-foreground py-8 text-center text-sm'>
          {t('Loading…')}
        </div>
      ) : null}

      {!loading && templates.length === 0 ? (
        <Alert>
          <AlertDescription>
            {t('No email templates available')}
          </AlertDescription>
        </Alert>
      ) : null}

      {templates.length > 0 ? (
        <div className='grid items-start gap-5 xl:grid-cols-[320px_minmax(0,1fr)]'>
          <Card className='xl:sticky xl:top-4'>
            <CardHeader className='border-b'>
              <CardTitle>{t('Template catalog')}</CardTitle>
              <CardDescription>
                {t('{{count}} templates', { count: templates.length })}
              </CardDescription>
            </CardHeader>
            <CardContent className='space-y-5'>
              {TEMPLATE_GROUPS.map((group) => {
                const groupTemplates = templates.filter(
                  (template) =>
                    (TEMPLATE_META[template.key]?.group || 'operations') ===
                    group.key
                )
                if (groupTemplates.length === 0) return null
                return (
                  <div key={group.key} className='space-y-1.5'>
                    <div className='text-muted-foreground px-2 text-xs font-medium tracking-wide uppercase'>
                      {t(group.label)}
                    </div>
                    {groupTemplates.map((template) => {
                      const meta = TEMPLATE_META[template.key]
                      const Icon = meta?.icon || ShieldCheck
                      const isActive = template.key === activeKey
                      return (
                        <button
                          key={template.key}
                          type='button'
                          onClick={() => setActiveKey(template.key)}
                          className={cn(
                            'hover:bg-muted/70 flex w-full items-start gap-3 rounded-lg px-2.5 py-2.5 text-left transition-colors',
                            isActive && 'bg-primary/8 ring-primary/20 ring-1'
                          )}
                        >
                          <span
                            className={cn(
                              'bg-muted mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg',
                              isActive && 'bg-primary/10 text-primary'
                            )}
                          >
                            <Icon className='size-4' />
                          </span>
                          <span className='min-w-0 flex-1'>
                            <span className='line-clamp-2 block text-sm font-medium'>
                              {t(meta?.name || template.name)}
                            </span>
                            <span className='text-muted-foreground mt-1 flex items-center gap-1.5 text-xs'>
                              {t(
                                AUDIENCE_LABELS[
                                  meta?.audience || 'notification'
                                ]
                              )}
                              {template.customized ? (
                                <>
                                  <span>·</span>
                                  <span className='text-primary'>
                                    {t('Customized')}
                                  </span>
                                </>
                              ) : null}
                            </span>
                          </span>
                        </button>
                      )
                    })}
                  </div>
                )
              })}
            </CardContent>
          </Card>

          {activeTemplate ? (
            <Card>
              <CardContent className='space-y-5'>
                <div className='overflow-hidden rounded-xl border'>
                  <div className='bg-muted/35 flex min-h-12 flex-wrap items-center justify-between gap-2 border-b px-4 py-2.5'>
                    <div className='min-w-0'>
                      <div className='text-sm font-medium'>
                        {t('Live preview')}
                      </div>
                      <div className='text-muted-foreground truncate text-xs'>
                        {previewSubject || t('Generating preview...')}
                      </div>
                    </div>
                    <Badge variant='outline'>
                      {inlinePreviewLoading
                        ? t('Updating...')
                        : t('Preview up to date')}
                    </Badge>
                  </div>
                  <iframe
                    title='inline-email-preview'
                    srcDoc={previewBody || ''}
                    sandbox=''
                    className='h-[460px] w-full bg-white'
                  />
                </div>

                <Separator />

                <Collapsible open={advancedOpen} onOpenChange={setAdvancedOpen}>
                  <CollapsibleTrigger
                    render={
                      <Button
                        type='button'
                        variant='outline'
                        className='w-full justify-between'
                      />
                    }
                  >
                    <span className='flex items-center gap-2'>
                      <Code2 className='size-4' />
                      {t('Advanced HTML editor')}
                    </span>
                    <ChevronDown
                      className={cn(
                        'size-4 transition-transform',
                        advancedOpen && 'rotate-180'
                      )}
                    />
                  </CollapsibleTrigger>
                  <CollapsibleContent className='space-y-5 pt-5'>
                    <Alert>
                      <AlertDescription>
                        {t(
                          'The default branded layout already adapts to the selected language. Edit HTML only when you need a custom structure or style.'
                        )}
                      </AlertDescription>
                    </Alert>

                    <div className='space-y-2'>
                      <div className='flex items-center justify-between gap-3'>
                        <Label>{t('Available variables')}</Label>
                        <span className='text-muted-foreground text-xs'>
                          {t('Click to insert at the cursor')}
                        </span>
                      </div>
                      <div className='grid gap-2 sm:grid-cols-2'>
                        {(activeTemplate.variables || []).map((variable) => (
                          <button
                            key={variable.name}
                            type='button'
                            onClick={() => insertVariable(variable.name)}
                            className='hover:bg-muted/60 flex min-w-0 items-center gap-2 rounded-lg border px-3 py-2 text-left transition-colors'
                          >
                            <code className='text-primary shrink-0 text-xs'>
                              {`{{${variable.name}}}`}
                            </code>
                            {variable.description ? (
                              <span className='text-muted-foreground truncate text-xs'>
                                {variable.description}
                              </span>
                            ) : null}
                          </button>
                        ))}
                      </div>
                    </div>

                    <div className='space-y-2'>
                      <Label htmlFor='email-template-body'>
                        {t('Email body (HTML)')}
                      </Label>
                      <Textarea
                        id='email-template-body'
                        ref={bodyRef}
                        value={currentDraft.body}
                        onChange={(event) =>
                          updateDraft({ body: event.target.value })
                        }
                        placeholder={activeTemplate.default_body}
                        rows={16}
                        className='font-mono text-xs leading-relaxed'
                      />
                    </div>
                  </CollapsibleContent>
                </Collapsible>
              </CardContent>
            </Card>
          ) : null}
        </div>
      ) : null}

      <EmailDeliveryFailures />

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
                {previewSubject || '(empty)'}
              </div>
            </div>
            <div className='space-y-1'>
              <Label className='text-muted-foreground text-xs font-normal'>
                {t('Body preview')}
              </Label>
              <iframe
                title='email-preview'
                srcDoc={previewBody || ''}
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

      <Dialog open={resetOpen} onOpenChange={setResetOpen}>
        <DialogContent className='sm:max-w-md'>
          <DialogHeader>
            <DialogTitle>{t('Restore defaults?')}</DialogTitle>
          </DialogHeader>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Both customized subject and body will be cleared and the built-in defaults will be used again.'
            )}
          </p>
          <DialogFooter>
            <Button variant='outline' onClick={() => setResetOpen(false)}>
              {t('Cancel')}
            </Button>
            <Button variant='destructive' onClick={handleReset}>
              {t('Restore defaults')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </SettingsSection>
  )
}

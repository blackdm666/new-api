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
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { FileText, FlaskConical, Send, WandSparkles } from 'lucide-react'
import { useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
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
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'
import { getUserFacingErrorMessage } from '@/lib/user-facing-error'

import { listNotices, submitNotice } from './api'
import { NoticeEmailPreview } from './email-preview'
import { NoticeRecords } from './notice-records'
import { NoticeRecipientPicker } from './recipient-picker'
import {
  noticeSchema,
  type NoticeContent,
  type NoticeRecord,
  type NoticeSubmission,
  type NoticeUser,
} from './types'

export function UserNotificationWorkbench(props: { preview?: boolean }) {
  const { t } = useTranslation()
  const preview = props.preview === true
  const request = useRef<{ signature: string; key: string } | null>(null)
  const queryClient = useQueryClient()
  const composer = useRef<HTMLDivElement>(null)
  const [selected, setSelected] = useState<NoticeUser[]>([])
  const [draftId, setDraftId] = useState<number>()
  const [pending, setPending] = useState<NoticeSubmission | null>(null)
  const [feedback, setFeedback] = useState('')
  const [failure, setFailure] = useState('')
  const form = useForm<NoticeContent>({
    resolver: zodResolver(noticeSchema),
    mode: 'onChange',
    defaultValues: { kind: 'maintenance', subject: '', body: '' },
  })
  const content = form.watch()
  const eligible = selected.filter((user) => !user.skip_reason)
  const history = useQuery({
    queryKey: ['user-notices', preview, 'notices'],
    queryFn: () => listNotices(preview),
    refetchInterval: preview ? false : 15000,
  })
  const submit = useMutation({
    mutationFn: (payload: NoticeSubmission) => submitNotice(payload, preview),
    meta: { errorToast: false },
    onSuccess: (record) => {
      request.current = null
      setFailure('')
      setPending(null)
      if (record.status === 'draft') {
        setDraftId(record.id)
        setFeedback(
          preview
            ? t(
                'Local draft saved. You can return to it from notification history.'
              )
            : t('Notification draft saved.')
        )
      } else {
        setDraftId(undefined)
        setSelected([])
        form.reset({ kind: 'maintenance', subject: '', body: '' })
        setFeedback(
          preview
            ? t(
                'Simulation completed. No real email was sent. See recipient results in notification history.'
              )
            : t(
                'Notification queued. SMTP acceptance does not prove inbox delivery; check the recipient results.'
              )
        )
      }
      void queryClient.invalidateQueries({
        queryKey: ['user-notices', preview, 'notices'],
      })
    },
    onError: (error) => {
      setPending(null)
      setFailure(getUserFacingErrorMessage(error))
    },
  })
  const blocked = submit.isPending || pending !== null
  const valid = form.formState.isValid && eligible.length > 0
  const useTemplate = () => {
    const templates: Record<
      NoticeContent['kind'],
      { subject: string; body: string }
    > = {
      maintenance: {
        subject: t('Scheduled service maintenance'),
        body: t('Maintenance notice example'),
      },
      usage: {
        subject: t('Please review your API usage'),
        body: t('Usage reminder example'),
      },
      violation: {
        subject: t('Account policy warning'),
        body: t('Policy warning example'),
      },
      service: {
        subject: t('An update about your account'),
        body: t('Service notice example'),
      },
    }
    form.setValue('subject', templates[content.kind].subject, {
      shouldValidate: true,
      shouldDirty: true,
    })
    form.setValue('body', templates[content.kind].body, {
      shouldValidate: true,
      shouldDirty: true,
    })
  }
  const prepare = (action: NoticeSubmission['action']) =>
    form.handleSubmit((values) => {
      if (eligible.length === 0 || blocked) return
      setFeedback('')
      setFailure('')
      const signature = JSON.stringify({
        ...values,
        id: draftId,
        user_ids: selected.map((user) => user.id).sort((a, b) => a - b),
        action,
      })
      if (request.current?.signature !== signature) {
        request.current = { signature, key: crypto.randomUUID() }
      }
      const payload: NoticeSubmission = {
        ...values,
        id: draftId,
        user_ids: selected.map((user) => user.id),
        action,
        request_key: request.current.key,
      }
      if (action === 'send') setPending(payload)
      else submit.mutate(payload)
    })
  const edit = (record: NoticeRecord) => {
    if (blocked) return
    setDraftId(record.id)
    setSelected(record.recipients)
    form.reset({
      kind: record.kind,
      subject: record.subject,
      body: record.body,
    })
    void form.trigger()
    setFeedback('')
    setFailure('')
    composer.current?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }

  return (
    <div className='space-y-5' ref={composer}>
      {preview && (
        <Alert className='border-amber-200 bg-amber-50/70 dark:border-amber-900 dark:bg-amber-950/30'>
          <FlaskConical aria-hidden='true' />
          <AlertTitle>
            {t('Local interactive preview — no emails will be sent')}
          </AlertTitle>
          <AlertDescription>
            {t(
              'All users and records are fictional. This preview does not connect to production or SMTP. Demo drafts are kept only until the local server restarts.'
            )}
          </AlertDescription>
        </Alert>
      )}
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <div>
          <h2 className='text-lg font-semibold'>
            {t('Send a user notification')}
          </h2>
          <p className='text-muted-foreground mt-1 text-sm'>
            {t(
              'For maintenance updates, usage reminders and account policy notices. Not a marketing campaign.'
            )}
          </p>
        </div>
        <Badge variant='outline'>{t('Verification and security SMTP')}</Badge>
      </div>
      {feedback && (
        <Alert role='status'>
          <AlertTitle>{t('Preview result')}</AlertTitle>
          <AlertDescription>{feedback}</AlertDescription>
        </Alert>
      )}
      {failure && (
        <Alert variant='destructive'>
          <AlertTitle>{t('Could not save notification')}</AlertTitle>
          <AlertDescription>{failure}</AlertDescription>
        </Alert>
      )}
      <div className='grid min-w-0 items-start gap-5 xl:grid-cols-[minmax(0,1.35fr)_minmax(320px,1fr)]'>
        <div className='min-w-0 space-y-5'>
          <NoticeRecipientPicker
            preview={preview}
            selected={selected}
            onChange={setSelected}
            disabled={blocked}
          />
          <Card>
            <CardHeader>
              <div className='flex items-center justify-between gap-2'>
                <CardTitle>{t('2. Write your notification')}</CardTitle>
                {draftId && (
                  <Badge variant='secondary'>
                    {t('Editing draft #{{id}}', { id: draftId })}
                  </Badge>
                )}
              </div>
              <CardDescription>
                {t(
                  'Choose a template as a starting point, then adapt it to the actual situation.'
                )}
              </CardDescription>
            </CardHeader>
            <CardContent>
              <form onSubmit={prepare('send')} className='space-y-5'>
                <FieldGroup>
                  <Field>
                    <FieldLabel htmlFor='notice-kind'>
                      {t('Notification type')}
                    </FieldLabel>
                    <div className='flex flex-wrap gap-2'>
                      <NativeSelect
                        id='notice-kind'
                        {...form.register('kind')}
                        disabled={blocked}
                        className='min-w-44 flex-1'
                      >
                        <NativeSelectOption value='maintenance'>
                          {t('Maintenance notice')}
                        </NativeSelectOption>
                        <NativeSelectOption value='usage'>
                          {t('Usage reminder')}
                        </NativeSelectOption>
                        <NativeSelectOption value='violation'>
                          {t('Policy warning')}
                        </NativeSelectOption>
                        <NativeSelectOption value='service'>
                          {t('Other service notice')}
                        </NativeSelectOption>
                      </NativeSelect>
                      <Button
                        type='button'
                        variant='outline'
                        onClick={useTemplate}
                        disabled={blocked}
                      >
                        <WandSparkles className='size-4' aria-hidden='true' />
                        {t('Use template')}
                      </Button>
                    </div>
                  </Field>
                  <Field data-invalid={Boolean(form.formState.errors.subject)}>
                    <FieldLabel htmlFor='notice-subject'>
                      {t('Subject')}
                    </FieldLabel>
                    <Input
                      id='notice-subject'
                      {...form.register('subject')}
                      maxLength={120}
                      disabled={blocked}
                      placeholder={t(
                        'A clear subject that tells the user what to do'
                      )}
                      aria-invalid={Boolean(form.formState.errors.subject)}
                    />
                    {form.formState.errors.subject && (
                      <FieldDescription>
                        {t('Enter a subject of 1–120 characters.')}
                      </FieldDescription>
                    )}
                  </Field>
                  <Field data-invalid={Boolean(form.formState.errors.body)}>
                    <FieldLabel htmlFor='notice-body'>
                      {t('Message')}
                    </FieldLabel>
                    <Textarea
                      id='notice-body'
                      {...form.register('body')}
                      maxLength={5000}
                      rows={9}
                      className='min-h-52 resize-y'
                      disabled={blocked}
                      placeholder={t(
                        'Explain what happened, what the user should do, and how to contact support.'
                      )}
                      aria-invalid={Boolean(form.formState.errors.body)}
                    />
                    <FieldDescription>
                      {t(
                        'Plain text only. No promotional content or sensitive credentials.'
                      )}
                    </FieldDescription>
                    {form.formState.errors.body && (
                      <FieldDescription>
                        {t('Enter a message of 1–5000 characters.')}
                      </FieldDescription>
                    )}
                  </Field>
                </FieldGroup>
                <div className='bg-muted/50 flex flex-wrap items-center justify-between gap-3 rounded-lg p-3'>
                  <p
                    className='text-muted-foreground text-xs'
                    aria-live='polite'
                  >
                    {t(
                      '{{eligible}} can receive · {{skipped}} will be skipped',
                      {
                        eligible: eligible.length,
                        skipped: selected.length - eligible.length,
                      }
                    )}
                  </p>
                  <div className='flex gap-2'>
                    <Button
                      type='button'
                      variant='outline'
                      disabled={!valid || blocked}
                      onClick={() => void prepare('draft')()}
                    >
                      <FileText className='size-4' aria-hidden='true' />
                      {t('Save draft')}
                    </Button>
                    <Button type='submit' disabled={!valid || blocked}>
                      <Send className='size-4' aria-hidden='true' />
                      {t('Review and send')}
                    </Button>
                  </div>
                </div>
              </form>
            </CardContent>
          </Card>
        </div>
        <aside className='min-w-0 space-y-4 xl:sticky xl:top-4'>
          <NoticeEmailPreview content={content} count={eligible.length} />
          <div className='text-muted-foreground px-2 text-xs leading-6'>
            <p>
              {t(
                'Disabled accounts may receive service notices; deleted accounts and blocked mailboxes must not be silently re-enabled.'
              )}
            </p>
            <p>
              {t(
                'Uses verification SMTP first, with the existing primary/backup fallback. Limited to 5 attempts per minute; verification codes take priority.'
              )}
            </p>
          </div>
        </aside>
      </div>
      {history.isError ? (
        <Alert variant='destructive'>
          <AlertTitle>{t('Could not load notification history')}</AlertTitle>
          <Button variant='outline' onClick={() => void history.refetch()}>
            {t('Retry')}
          </Button>
        </Alert>
      ) : (
        <NoticeRecords
          records={history.data ?? []}
          onEdit={edit}
          preview={preview}
        />
      )}
      <ConfirmDialog
        open={pending !== null}
        onOpenChange={(open) => {
          if (!open && !submit.isPending) setPending(null)
        }}
        title={t('Confirm user notification')}
        confirmText={
          preview
            ? t('Confirm simulated send')
            : t('Confirm and queue notification')
        }
        isLoading={submit.isPending}
        handleConfirm={() => {
          if (pending && !submit.isPending) submit.mutate(pending)
        }}
        desc={
          preview
            ? t(
                'This is a local simulation. It will record a result without contacting any mailbox.'
              )
            : t(
                'This will send real service emails to the selected users using verification SMTP first. Check the content and recipients carefully.'
              )
        }
      >
        <div className='space-y-3 text-sm'>
          <p className='font-medium break-words'>{pending?.subject}</p>
          <p>
            {t('{{eligible}} can receive · {{skipped}} will be skipped', {
              eligible: eligible.length,
              skipped: selected.length - eligible.length,
            })}
          </p>
          <ul className='max-h-52 space-y-2 overflow-y-auto rounded-lg border p-3'>
            {selected.map((user) => (
              <li
                key={user.id}
                className='flex flex-wrap justify-between gap-2'
              >
                <span>
                  {user.display_name} · UID {user.id}
                </span>
                <span className='text-muted-foreground'>
                  {user.skip_reason ? t('Will be skipped') : user.email_masked}
                </span>
              </li>
            ))}
          </ul>
        </div>
      </ConfirmDialog>
    </div>
  )
}

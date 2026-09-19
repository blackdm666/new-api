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
import { Mail, ShieldCheck } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'

import type { NoticeContent } from './types'

export function NoticeEmailPreview(props: {
  content: NoticeContent
  count: number
}) {
  const { t } = useTranslation()
  return (
    <Card className='overflow-hidden'>
      <CardHeader>
        <div className='flex items-center justify-between gap-2'>
          <CardTitle className='flex items-center gap-2'>
            <Mail className='size-4' aria-hidden='true' />
            {t('Email preview')}
          </CardTitle>
          <Badge variant='outline'>{t('Service notice')}</Badge>
        </div>
        <CardDescription>
          {t(
            'A separate email for each recipient. No wallet button or marketing tracking.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='space-y-5'>
        <div className='bg-background overflow-hidden rounded-xl border'>
          <div className='bg-slate-900 px-6 py-5 text-white'>
            <div className='text-lg font-semibold tracking-tight'>
              88API{' '}
              <span className='ml-2 text-xs font-normal text-slate-300'>
                {t('User notifications')}
              </span>
            </div>
          </div>
          <article className='space-y-5 p-6'>
            <h3 className='text-lg font-semibold break-words'>
              {props.content.subject || t('Your notice subject')}
            </h3>
            <p className='text-muted-foreground text-sm leading-7 break-words whitespace-pre-wrap'>
              {props.content.body ||
                t('Write your message to preview the email here.')}
            </p>
            <Separator />
            <p className='text-muted-foreground text-xs leading-5'>
              {t(
                'This is an account or service notice, not a promotional email. If you have questions, contact support through the website.'
              )}
            </p>
          </article>
        </div>
        <div className='text-muted-foreground flex items-start gap-2 text-xs leading-6'>
          <ShieldCheck className='mt-1 size-4 shrink-0' aria-hidden='true' />
          <p>
            {t(
              '{{count}} eligible recipients. Recipient addresses are never shared with other users.',
              { count: props.count }
            )}
          </p>
        </div>
      </CardContent>
    </Card>
  )
}

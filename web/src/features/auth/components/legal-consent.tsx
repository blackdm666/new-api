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
import { useTranslation } from 'react-i18next'

import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { cn } from '@/lib/utils'

import type { SystemStatus } from '../types'

interface LegalConsentProps {
  status: SystemStatus | null
  checked: boolean
  onCheckedChange: (nextValue: boolean) => void
  className?: string
}

export function LegalConsent({
  status,
  checked,
  onCheckedChange,
  className,
}: LegalConsentProps) {
  const { t, i18n } = useTranslation()
  const hasUserAgreement = Boolean(status?.user_agreement_enabled)
  const hasPrivacyPolicy = Boolean(status?.privacy_policy_enabled)
  const isChinese = i18n.resolvedLanguage?.toLowerCase().startsWith('zh')

  if (!hasUserAgreement && !hasPrivacyPolicy) {
    return null
  }

  const handleChange = (value: boolean) => {
    onCheckedChange(value === true)
  }

  return (
    <div
      className={cn(
        'border-border/60 bg-muted/40 flex items-start gap-3 rounded-md border p-3 transition-[border-color,box-shadow]',
        !checked &&
          'border-blue-500/70 shadow-[0_0_0_1px_rgb(59_130_246_/_0.30),0_0_18px_rgb(37_99_235_/_0.22)]',
        className
      )}
    >
      <Checkbox
        id='legal-consent'
        checked={checked}
        onCheckedChange={handleChange}
        className='mt-0.5'
      />
      <Label
        htmlFor='legal-consent'
        className='text-muted-foreground min-w-0 flex-1 items-start gap-1 text-left text-xs leading-5 font-normal'
      >
        <span>
          {t('I have read and agree to the')}
          {isChinese ? (
            <>
              {hasUserAgreement && (
                <>
                  《
                  <a
                    href='/user-agreement'
                    target='_blank'
                    rel='noopener noreferrer'
                    className='text-primary hover:underline'
                  >
                    {t('User Agreement')}
                  </a>
                  》
                </>
              )}
              {hasUserAgreement && hasPrivacyPolicy && '和'}
              {hasPrivacyPolicy && (
                <>
                  《
                  <a
                    href='/privacy-policy'
                    target='_blank'
                    rel='noopener noreferrer'
                    className='text-primary hover:underline'
                  >
                    {t('Privacy Policy')}
                  </a>
                  》
                </>
              )}
            </>
          ) : (
            <>
              {' '}
              {hasUserAgreement && (
                <a
                  href='/user-agreement'
                  target='_blank'
                  rel='noopener noreferrer'
                  className='text-primary hover:underline'
                >
                  {t('User Agreement')}
                </a>
              )}
              {hasUserAgreement && hasPrivacyPolicy && ' and the '}
              {hasPrivacyPolicy && (
                <a
                  href='/privacy-policy'
                  target='_blank'
                  rel='noopener noreferrer'
                  className='text-primary hover:underline'
                >
                  {t('Privacy Policy')}
                </a>
              )}
              .
            </>
          )}
        </span>
      </Label>
      {!checked && (
        <span className='shrink-0 rounded-full border border-blue-400/60 bg-blue-950/20 px-2 py-1 text-[11px] leading-none font-semibold text-blue-500 dark:text-blue-300'>
          ✓ {t('Required')}
        </span>
      )}
    </div>
  )
}

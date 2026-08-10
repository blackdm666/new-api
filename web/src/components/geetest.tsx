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
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import type { GeeTestValidation } from '@/features/auth/types'
import { cn } from '@/lib/utils'

interface GeeTestInstance {
  appendTo: (target: HTMLElement) => void
  destroy?: () => void
  getValidate: () => GeeTestValidation | false
  onReady: (callback: () => void) => GeeTestInstance
  onSuccess: (callback: () => void) => GeeTestInstance
  onFail: (callback: () => void) => GeeTestInstance
  onError: (callback: () => void) => GeeTestInstance
  onClose: (callback: () => void) => GeeTestInstance
}

interface GeeTestOptions {
  captchaId: string
  product: 'float'
  language: string
  protocol: 'https://'
  timeout: number
  nativeButton: {
    width: string
    height: string
  }
}

declare global {
  interface Window {
    initGeetest4?: (
      options: GeeTestOptions,
      callback: (captcha: GeeTestInstance) => void
    ) => void
  }
}

interface GeeTestProps {
  captchaId: string
  onVerify: (validation: GeeTestValidation | undefined) => void
  className?: string
}

let geeTestScriptPromise: Promise<void> | undefined

function loadGeeTestScript(): Promise<void> {
  if (window.initGeetest4) return Promise.resolve()
  if (geeTestScriptPromise) return geeTestScriptPromise

  const scriptPromise = new Promise<void>((resolve, reject) => {
    const existing = document.querySelector('#geetest-v4')
    const handleLoad = () => {
      if (window.initGeetest4) {
        resolve()
      } else {
        reject(new Error('GeeTest did not initialize'))
      }
    }
    if (existing) {
      existing.addEventListener('load', handleLoad, { once: true })
      existing.addEventListener(
        'error',
        () => reject(new Error('GeeTest failed to load')),
        {
          once: true,
        }
      )
      return
    }

    const script = document.createElement('script')
    script.id = 'geetest-v4'
    script.src = 'https://static.geetest.com/v4/gt4.js'
    script.async = true
    script.addEventListener('load', handleLoad, { once: true })
    script.addEventListener(
      'error',
      () => reject(new Error('GeeTest failed to load')),
      { once: true }
    )
    document.head.appendChild(script)
  }).catch((error: unknown) => {
    geeTestScriptPromise = undefined
    throw error
  })
  geeTestScriptPromise = scriptPromise
  return scriptPromise
}

function geeTestLanguage(language: string): string {
  const normalized = language.toLowerCase()
  if (normalized.startsWith('zh-tw')) return 'zho-tw'
  if (normalized.startsWith('zh-hk')) return 'zho-hk'
  if (normalized.startsWith('zh')) return 'zho'
  if (normalized.startsWith('ja')) return 'jpn'
  if (normalized.startsWith('ko')) return 'kor'
  if (normalized.startsWith('ru')) return 'rus'
  if (normalized.startsWith('fr')) return 'fra'
  if (normalized.startsWith('vi')) return 'eng'
  return 'eng'
}

export function GeeTest({ captchaId, onVerify, className }: GeeTestProps) {
  const { t, i18n } = useTranslation()
  const containerRef = useRef<HTMLDivElement | null>(null)
  const instanceRef = useRef<GeeTestInstance | undefined>(undefined)
  const [state, setState] = useState<'loading' | 'ready' | 'error'>('loading')

  useEffect(() => {
    let cancelled = false
    setState('loading')
    onVerify(undefined)

    loadGeeTestScript()
      .then(() => {
        if (cancelled || !window.initGeetest4 || !containerRef.current) return
        window.initGeetest4(
          {
            captchaId,
            product: 'float',
            language: geeTestLanguage(i18n.resolvedLanguage || i18n.language),
            protocol: 'https://',
            timeout: 10000,
            nativeButton: { width: '100%', height: '44px' },
          },
          (captcha) => {
            if (cancelled || !containerRef.current) {
              captcha.destroy?.()
              return
            }
            instanceRef.current = captcha
            captcha
              .onReady(() => setState('ready'))
              .onSuccess(() => {
                const validation = captcha.getValidate()
                onVerify(validation || undefined)
              })
              .onFail(() => onVerify(undefined))
              .onError(() => {
                onVerify(undefined)
                setState('error')
              })
              .onClose(() => onVerify(undefined))
            captcha.appendTo(containerRef.current)
          }
        )
      })
      .catch(() => {
        if (!cancelled) setState('error')
      })

    return () => {
      cancelled = true
      instanceRef.current?.destroy?.()
      instanceRef.current = undefined
    }
  }, [captchaId, i18n.language, i18n.resolvedLanguage, onVerify])

  return (
    <div
      className={cn('space-y-2', className)}
      aria-label={t('Security verification')}
    >
      <div ref={containerRef} />
      {state === 'loading' && (
        <p className='text-muted-foreground text-xs'>{t('Loading...')}</p>
      )}
      {state === 'error' && (
        <p className='text-destructive text-xs'>{t('Verification failed')}</p>
      )}
    </div>
  )
}

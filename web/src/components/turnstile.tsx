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
import { useEffect, useRef } from 'react'

import {
  resolveTurnstileWidgetEndpoint,
  type TurnstileClientConfig,
} from '@/components/turnstile-utils'

declare global {
  interface Window {
    Captcha88?: {
      render: (options: {
        el: HTMLElement | string
        endpoint?: string
        act?: string
        siteKey?: string
        onToken?: (token: string) => void
        onError?: (message: string) => void
      }) => unknown
    }
    turnstile?: {
      render: (
        element: HTMLElement,
        options: Record<string, unknown>
      ) => string | number | undefined
      remove?: (widgetId: string | number) => void
    }
  }
}

interface TurnstileProps extends TurnstileClientConfig {
  onVerify: (token: string) => void
  onExpire?: () => void
  className?: string
}

export function Turnstile({
  provider,
  siteKey,
  widgetScriptURL,
  widgetEndpoint,
  action,
  onVerify,
  onExpire,
  className,
}: TurnstileProps) {
  const ref = useRef<HTMLDivElement | null>(null)
  const rendered = useRef(false)
  const onVerifyRef = useRef(onVerify)
  const onExpireRef = useRef(onExpire)

  useEffect(() => {
    onVerifyRef.current = onVerify
    onExpireRef.current = onExpire
  }, [onExpire, onVerify])

  useEffect(() => {
    rendered.current = false
    const element = ref.current
    const custom = provider === 'custom'
    const renderedWidgetEndpoint = resolveTurnstileWidgetEndpoint(
      widgetEndpoint,
      custom &&
        import.meta.env.DEV &&
        import.meta.env.VITE_CAPTCHA_DEV_PROXY_ENABLED === 'true'
    )
    const scriptURL = custom
      ? widgetScriptURL
      : 'https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit'
    let disposed = false
    let removeScriptListeners: (() => void) | undefined
    let removeWidget: (() => void) | undefined

    const expire = () => onExpireRef.current?.()

    const cleanup = () => {
      disposed = true
      removeScriptListeners?.()
      try {
        removeWidget?.()
      } catch {
        // The container cleanup below is the compatibility fallback for
        // custom widgets that do not expose a teardown API.
      }
      element?.replaceChildren()
      rendered.current = false
    }

    const render = () => {
      if (!element || rendered.current || disposed) return

      try {
        if (custom && window.Captcha88) {
          const widget = window.Captcha88.render({
            el: element,
            endpoint: renderedWidgetEndpoint,
            act: action,
            ...(siteKey ? { siteKey } : {}),
            onToken: (token: string) => onVerifyRef.current(token),
            onError: expire,
          })
          if (typeof widget === 'function') {
            removeWidget = widget as () => void
          } else if (widget && typeof widget === 'object') {
            const handle = widget as {
              destroy?: () => void
              remove?: () => void
            }
            if (typeof handle.destroy === 'function') {
              removeWidget = () => handle.destroy?.()
            } else if (typeof handle.remove === 'function') {
              removeWidget = () => handle.remove?.()
            }
          }
          rendered.current = true
        } else if (!custom && window.turnstile) {
          const widgetId = window.turnstile.render(element, {
            sitekey: siteKey,
            callback: (token: string) => onVerifyRef.current(token),
            'error-callback': expire,
            'expired-callback': expire,
          })
          if (widgetId !== undefined) {
            removeWidget = () => window.turnstile?.remove?.(widgetId)
          }
          rendered.current = true
        }
      } catch {
        expire()
      }
    }

    if (!scriptURL) {
      expire()
      return cleanup
    }

    const scriptId = custom ? 'custom-turnstile-widget' : 'cf-turnstile'
    let existing = document.querySelector<HTMLScriptElement>(`#${scriptId}`)
    let replacedMismatchedScript = false
    if (
      existing &&
      existing.src !== new URL(scriptURL, window.location.href).href
    ) {
      existing.remove()
      existing = null
      replacedMismatchedScript = true
    }

    if (
      (!custom && window.turnstile) ||
      (custom && window.Captcha88 && !replacedMismatchedScript)
    ) {
      render()
      return cleanup
    }

    if (existing) {
      const handleScriptError = expire
      existing.addEventListener('load', render)
      existing.addEventListener('error', handleScriptError)
      removeScriptListeners = () => {
        existing.removeEventListener('load', render)
        existing.removeEventListener('error', handleScriptError)
      }
      return cleanup
    }

    const s = document.createElement('script')
    s.id = scriptId
    s.src = scriptURL
    s.async = true
    s.defer = true
    const handleScriptError = expire
    s.addEventListener('load', render)
    s.addEventListener('error', handleScriptError)
    document.head.appendChild(s)
    removeScriptListeners = () => {
      s.removeEventListener('load', render)
      s.removeEventListener('error', handleScriptError)
    }
    return cleanup
  }, [action, provider, siteKey, widgetEndpoint, widgetScriptURL])

  return <div ref={ref} className={className} />
}

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

declare global {
  interface Window {
    Captcha88?: {
      render: (opts: {
        el: HTMLElement | string
        endpoint?: string
        act?: string
        onToken?: (token: string) => void
        onError?: (msg: string) => void
      }) => unknown
    }
  }
}

interface TurnstileProps {
  siteKey: string
  onVerify: (token: string) => void
  onExpire?: () => void
  className?: string
}

// 88API 自研人机验证组件(伪装 Cloudflare 外观:勾选 → 行为/点击/滑块)。
// 复用 new-api 现成的 Turnstile 接线:后台“Turnstile 站点密钥”填我方验证服务地址
// (如 https://verify.88api.ai),组件据此加载 widget.js 并渲染。校验通过后通过
// onVerify 回传一次性 pass_token,后端由 TurnstileCheck 中间件核销。
// 组件对外签名与原 Cloudflare 版保持一致,表单/hook/api 层无需改动。
export function Turnstile({
  siteKey,
  onVerify,
  onExpire,
  className,
}: TurnstileProps) {
  const ref = useRef<HTMLDivElement | null>(null)
  const rendered = useRef(false)

  useEffect(() => {
    const endpoint =
      (siteKey || '').replace(/\/+$/, '') || 'https://verify.88api.ai'

    const render = () => {
      if (!ref.current || rendered.current || !window.Captcha88) return
      rendered.current = true
      try {
        window.Captcha88.render({
          el: ref.current,
          endpoint,
          act: 'register',
          onToken: (token: string) => onVerify(token),
          onError: () => onExpire?.(),
        })
      } catch {
        /* empty */
      }
    }

    if (window.Captcha88) {
      render()
      return
    }
    const scriptId = 'c88-widget'
    const existing = document.getElementById(scriptId) as HTMLScriptElement | null
    if (existing) {
      existing.addEventListener('load', render)
      return
    }
    const s = document.createElement('script')
    s.id = scriptId
    s.src = `${endpoint}/widget.js`
    s.async = true
    s.defer = true
    s.onload = () => render()
    document.head.appendChild(s)
  }, [siteKey, onVerify, onExpire])

  return <div ref={ref} className={className} />
}

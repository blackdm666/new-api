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
import { Info } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { getCurrencyDisplay } from '@/lib/currency'
import { handleServerError } from '@/lib/handle-server-error'

import { enableApiKeyUserQuotaQuery } from '../../api'
import {
  buildCCSwitchUsageScript,
  CC_SWITCH_USAGE_INTERVAL_MINUTES,
  encodeCCSwitchUsageScript,
  type CCSwitchUsageUnit,
} from '../../lib/cc-switch-usage'
import type { ApiKey } from '../../types'

type CCSwitchApp = 'claude' | 'codex' | 'gemini' | 'grokbuild'

// Claude Desktop keeps its own provider list in CC Switch but cannot be
// imported by link, so the dialog walks users through the in-app preset.
type DialogApp = CCSwitchApp | 'claude-desktop'

type CCSwitchAppPreset = {
  // CC Switch uses the first endpoint as the base URL and saves the rest as
  // speed-test candidates, like its preset's endpointCandidates.
  endpoints: readonly string[]
  // Default model only; any model in the key's group stays usable.
  model?: string
  // Models the key's group must offer for this app to work.
  modelFamily: string
}

// Mirrors the built-in 88API presets of CC Switch v4.0+ (src/config/
// *ProviderPresets.ts). ccswitch:// links cannot reference a preset, so every
// preset field is sent explicitly. Codex and Grok Build imports always use
// the Responses API. CC Switch rejects provider links for Claude Desktop, Pi
// and MiniMax Code, and links for OpenCode, OpenClaw and Hermes are forced
// onto an OpenAI-compatible protocol with one model, unlike their presets, so
// those apps are pointed at the in-app preset instead.
const CC_SWITCH_PROVIDER = {
  name: '88API',
  homepage: 'https://88api.ai',
  icon: '88api',
} as const

const CC_SWITCH_APPS: Record<CCSwitchApp, CCSwitchAppPreset> = {
  claude: {
    endpoints: ['https://api.88api.ai', 'https://88api.ai'],
    modelFamily: 'Claude',
  },
  codex: {
    endpoints: ['https://api.88api.ai/v1', 'https://88api.ai/v1'],
    // Newer than the preset's default model (gpt-5.6-sol) by choice.
    model: 'gpt-6.1-sol',
    modelFamily: 'GPT',
  },
  gemini: {
    endpoints: ['https://api.88api.ai', 'https://88api.ai'],
    model: 'gemini-3.8-flash',
    modelFamily: 'Gemini',
  },
  grokbuild: {
    endpoints: ['https://api.88api.ai/v1', 'https://88api.ai/v1'],
    // Newer than the preset's default model (grok-4.5) by choice.
    model: 'grok-4.7',
    modelFamily: 'Grok',
  },
}

// Option order: the two Claude apps share the first row.
const APP_LABELS: Record<DialogApp, string> = {
  claude: 'Claude Code CLI',
  'claude-desktop': 'Claude Desktop',
  codex: 'Codex',
  gemini: 'Gemini CLI',
  grokbuild: 'Grok Build',
}

function buildCCSwitchURL(
  app: CCSwitchApp,
  apiKey: string,
  usageScript?: string
): string {
  const preset = CC_SWITCH_APPS[app]
  const params = new URLSearchParams()
  params.set('resource', 'provider')
  params.set('app', app)
  params.set('name', CC_SWITCH_PROVIDER.name)
  params.set('icon', CC_SWITCH_PROVIDER.icon)
  params.set('homepage', CC_SWITCH_PROVIDER.homepage)
  params.set('endpoint', preset.endpoints.join(','))
  params.set('apiKey', apiKey)
  if (preset.model) params.set('model', preset.model)
  params.set('enabled', 'true')
  if (usageScript) {
    params.set('usageScript', encodeCCSwitchUsageScript(usageScript))
    // usageEnabled keeps CC Switch releases before 4.0.10 from ignoring it.
    params.set('usageEnabled', 'true')
    params.set('usageAutoInterval', String(CC_SWITCH_USAGE_INTERVAL_MINUTES))
  }
  return `ccswitch://v1/import?${params.toString()}`
}

// The balance shown in CC Switch uses the console's display currency; a
// token-count display falls back to USD, as billing amounts do.
function getCCSwitchUsageUnit(): CCSwitchUsageUnit {
  const { config, meta } = getCurrencyDisplay()
  if (meta.kind === 'tokens') {
    return { quotaPerUnit: config.quotaPerUnit, unit: 'USD' }
  }
  return {
    quotaPerUnit: config.quotaPerUnit / meta.exchangeRate,
    unit: meta.kind === 'currency' ? meta.currencyCode : meta.symbol,
  }
}

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  tokenKey: string
  group: string
  // The key being imported; when it does not yet allow user quota queries,
  // importing with the balance shown turns that on.
  apiKey?: ApiKey | null
  onBalanceQueryEnabled?: () => void
}

export function CCSwitchDialog(props: Props) {
  const { t } = useTranslation()
  const { copyToClipboard } = useCopyToClipboard()
  const [app, setApp] = useState<DialogApp>('claude')
  const [showBalance, setShowBalance] = useState(true)

  useEffect(() => {
    if (props.open) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setApp('claude')
      setShowBalance(true)
    }
  }, [props.open])

  const key = props.tokenKey.startsWith('sk-')
    ? props.tokenKey
    : `sk-${props.tokenKey}`
  const preset = app === 'claude-desktop' ? null : CC_SWITCH_APPS[app]
  let groupLabel = props.group.trim()
  if (!groupLabel) groupLabel = t('Follow user group')
  if (groupLabel === 'auto') groupLabel = t('Cross-group')

  const balanceAllowed = props.apiKey?.allow_user_quota_query === true

  const handleSubmit = async () => {
    if (app === 'claude-desktop') {
      await copyToClipboard(key)
      return
    }
    const usageScript = showBalance
      ? buildCCSwitchUsageScript(getCCSwitchUsageUnit(), {
          wallet: t('Wallet balance'),
          key: t('Key quota'),
          disabled: t(
            'Turn on "Allow querying user quota" for this key in the console to see the wallet balance'
          ),
          failed: t('Balance query failed'),
        })
      : undefined
    // Start allowing balance queries before CC Switch opens, but open it in
    // this click so the browser does not block it. The user still has to
    // confirm the import in CC Switch, so the change lands before its first
    // balance query.
    const enabling =
      showBalance && props.apiKey && !balanceAllowed
        ? enableApiKeyUserQuotaQuery(props.apiKey)
        : null
    window.open(buildCCSwitchURL(app, key, usageScript), '_blank')
    props.onOpenChange(false)
    if (!enabling) return
    try {
      const result = await enabling
      if (!result.success) {
        handleServerError(
          result,
          t('Failed to allow balance queries for this key')
        )
        return
      }
      props.onBalanceQueryEnabled?.()
    } catch (error) {
      handleServerError(
        error,
        t('Failed to allow balance queries for this key')
      )
    }
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Import to CC Switch')}
      description={t(
        "Imports this API key with the same settings as CC Switch's built-in 88API preset."
      )}
      contentClassName='sm:max-w-md'
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button variant='outline' onClick={() => props.onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button onClick={handleSubmit}>
            {preset ? t('Open CC Switch') : t('Copy Key')}
          </Button>
        </>
      }
    >
      <div className='space-y-2'>
        <div className='flex items-center gap-1'>
          <Label id='cc-switch-app-label'>{t('Application')}</Label>
          <Popover>
            <PopoverTrigger
              render={
                <Button
                  type='button'
                  variant='ghost'
                  size='icon-xs'
                  aria-label={t('Other apps')}
                  className='text-muted-foreground'
                />
              }
            >
              <Info aria-hidden='true' />
            </PopoverTrigger>
            <PopoverContent className='w-72 text-xs leading-relaxed'>
              {t(
                "OpenCode, OpenClaw, Hermes, Pi and MiniMax Code can't be imported by link. In CC Switch, open that app, choose Add Provider, pick the built-in 88API preset and paste this API key."
              )}
            </PopoverContent>
          </Popover>
        </div>
        <RadioGroup
          aria-labelledby='cc-switch-app-label'
          value={app}
          onValueChange={(value) => setApp(value as DialogApp)}
          className='grid grid-cols-2 gap-x-4 gap-y-2'
        >
          {(Object.keys(APP_LABELS) as DialogApp[]).map((value) => (
            <div key={value} className='flex items-center gap-2'>
              <RadioGroupItem value={value} id={`app-${value}`} />
              <Label htmlFor={`app-${value}`} className='cursor-pointer'>
                {APP_LABELS[value]}
              </Label>
            </div>
          ))}
        </RadioGroup>
      </div>

      {preset ? (
        <div className='space-y-1.5'>
          <dl className='space-y-2 rounded-md border p-3 text-sm'>
            <div className='flex justify-between gap-4'>
              <dt className='text-muted-foreground'>{t('Name')}</dt>
              <dd className='font-medium'>{CC_SWITCH_PROVIDER.name}</dd>
            </div>
            <div className='flex justify-between gap-4'>
              <dt className='text-muted-foreground shrink-0'>
                {t('Base URL')}
              </dt>
              <dd className='font-mono break-all'>{preset.endpoints[0]}</dd>
            </div>
            {preset.model && (
              <div className='flex justify-between gap-4'>
                <dt className='text-muted-foreground'>{t('Default model')}</dt>
                <dd className='font-mono'>{preset.model}</dd>
              </div>
            )}
            <div className='flex justify-between gap-4'>
              <dt className='text-muted-foreground shrink-0'>{t('Group')}</dt>
              <dd className='font-medium break-all'>{groupLabel}</dd>
            </div>
          </dl>
          {preset.model && (
            <p className='text-muted-foreground text-xs'>
              {t(
                "The default model is only the initial choice. You can switch to any model in the key's group later."
              )}
            </p>
          )}
        </div>
      ) : (
        <div className='space-y-2 rounded-md border p-3 text-sm'>
          <p className='font-medium'>
            {t(
              "Claude Desktop can't be imported by link. Add it in CC Switch:"
            )}
          </p>
          <ol className='text-muted-foreground list-decimal space-y-1 pl-5'>
            <li>
              {t(
                'In CC Switch, open Claude Desktop and choose Add Provider. Under third-party providers, 88API is the first preset.'
              )}
            </li>
            <li>
              {t('Click Copy Key below, paste it into API Key and save.')}
            </li>
          </ol>
          <dl className='flex justify-between gap-4 border-t pt-2'>
            <dt className='text-muted-foreground shrink-0'>{t('Group')}</dt>
            <dd className='font-medium break-all'>{groupLabel}</dd>
          </dl>
        </div>
      )}

      {preset && (
        <div className='flex items-start gap-2'>
          <Checkbox
            id='cc-switch-show-balance'
            checked={showBalance}
            onCheckedChange={(checked) => setShowBalance(checked === true)}
            className='mt-0.5'
          />
          <div className='space-y-0.5'>
            <Label htmlFor='cc-switch-show-balance' className='cursor-pointer'>
              {t('Show account balance in CC Switch')}
            </Label>
            <p className='text-muted-foreground text-xs'>
              {balanceAllowed
                ? t(
                    'CC Switch reads the wallet balance with this key and refreshes it every {{minutes}} minutes.',
                    { minutes: CC_SWITCH_USAGE_INTERVAL_MINUTES }
                  )
                : t(
                    'Also turns on "Allow querying user quota" for this key, so CC Switch can read the wallet balance with it.'
                  )}
            </p>
          </div>
        </div>
      )}

      <Alert>
        <AlertTitle>{t('Check the key group')}</AlertTitle>
        <AlertDescription>
          {t(
            '{{app}} needs a key in a group that offers {{family}} models, otherwise requests fail. Edit the key to change its group.',
            {
              app: APP_LABELS[app],
              family: preset ? preset.modelFamily : 'Claude',
            }
          )}
        </AlertDescription>
      </Alert>
    </Dialog>
  )
}

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
import { Label } from '@/components/ui/label'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'

type CCSwitchApp = 'claude' | 'codex' | 'gemini' | 'grokbuild'

type CCSwitchAppPreset = {
  label: string
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
    label: 'Claude Code',
    endpoints: ['https://api.88api.ai', 'https://88api.ai'],
    modelFamily: 'Claude',
  },
  codex: {
    label: 'Codex',
    endpoints: ['https://api.88api.ai/v1', 'https://88api.ai/v1'],
    // Newer than the preset's default model (gpt-5.6-sol) by choice.
    model: 'gpt-6.1-sol',
    modelFamily: 'GPT',
  },
  gemini: {
    label: 'Gemini CLI',
    endpoints: ['https://api.88api.ai', 'https://88api.ai'],
    model: 'gemini-3.8-flash',
    modelFamily: 'Gemini',
  },
  grokbuild: {
    label: 'Grok Build',
    endpoints: ['https://api.88api.ai/v1', 'https://88api.ai/v1'],
    // Newer than the preset's default model (grok-4.5) by choice.
    model: 'grok-4.7',
    modelFamily: 'Grok',
  },
}

function buildCCSwitchURL(app: CCSwitchApp, apiKey: string): string {
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
  return `ccswitch://v1/import?${params.toString()}`
}

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  tokenKey: string
  group: string
}

export function CCSwitchDialog(props: Props) {
  const { t } = useTranslation()
  const [app, setApp] = useState<CCSwitchApp>('claude')

  useEffect(() => {
    if (props.open) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setApp('claude')
    }
  }, [props.open])

  const preset = CC_SWITCH_APPS[app]
  let groupLabel = props.group.trim()
  if (!groupLabel) groupLabel = t('Follow user group')
  if (groupLabel === 'auto') groupLabel = t('Cross-group')

  const handleSubmit = () => {
    const key = props.tokenKey.startsWith('sk-')
      ? props.tokenKey
      : `sk-${props.tokenKey}`
    window.open(buildCCSwitchURL(app, key), '_blank')
    props.onOpenChange(false)
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
          <Button onClick={handleSubmit}>{t('Open CC Switch')}</Button>
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
                "OpenCode, OpenClaw, Hermes, Claude Desktop, Pi and MiniMax Code can't be imported by link. In CC Switch, open that app, choose Add Provider, pick the built-in 88API preset and paste this API key."
              )}
            </PopoverContent>
          </Popover>
        </div>
        <RadioGroup
          aria-labelledby='cc-switch-app-label'
          value={app}
          onValueChange={(value) => setApp(value as CCSwitchApp)}
          className='grid grid-cols-2 gap-x-4 gap-y-2'
        >
          {(Object.keys(CC_SWITCH_APPS) as CCSwitchApp[]).map((key) => (
            <div key={key} className='flex items-center gap-2'>
              <RadioGroupItem value={key} id={`app-${key}`} />
              <Label htmlFor={`app-${key}`} className='cursor-pointer'>
                {CC_SWITCH_APPS[key].label}
              </Label>
            </div>
          ))}
        </RadioGroup>
      </div>

      <div className='space-y-1.5'>
        <dl className='space-y-2 rounded-md border p-3 text-sm'>
          <div className='flex justify-between gap-4'>
            <dt className='text-muted-foreground'>{t('Name')}</dt>
            <dd className='font-medium'>{CC_SWITCH_PROVIDER.name}</dd>
          </div>
          <div className='flex justify-between gap-4'>
            <dt className='text-muted-foreground shrink-0'>{t('Base URL')}</dt>
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
        {app === 'claude' && (
          <p className='text-muted-foreground text-xs'>
            {t(
              'Adds the provider to Claude Code (CLI) only. Claude Desktop does not see it; add the 88API preset there separately in CC Switch.'
            )}
          </p>
        )}
      </div>

      <Alert>
        <AlertTitle>{t('Check the key group')}</AlertTitle>
        <AlertDescription>
          {t(
            '{{app}} needs a key in a group that offers {{family}} models, otherwise requests fail. Edit the key to change its group.',
            { app: preset.label, family: preset.modelFamily }
          )}
        </AlertDescription>
      </Alert>
    </Dialog>
  )
}

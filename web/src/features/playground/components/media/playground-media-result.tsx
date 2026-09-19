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
import { AlertCircle, ImageIcon, LoaderCircle, VideoIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'

import type { MediaTestState, PlaygroundModelMode } from '../../types'

type PlaygroundMediaResultProps = {
  mode: Exclude<PlaygroundModelMode, 'chat'>
  result: MediaTestState
}

export function PlaygroundMediaResult({
  mode,
  result,
}: PlaygroundMediaResultProps) {
  const { t } = useTranslation()
  const Icon = mode === 'image' ? ImageIcon : VideoIcon

  if (result.status === 'idle') {
    return (
      <div className='flex min-h-[min(520px,calc(100svh-18rem))] items-center justify-center px-4 py-12'>
        <div className='grid max-w-lg gap-3 text-center'>
          <div className='bg-muted/50 text-muted-foreground mx-auto flex size-11 items-center justify-center rounded-xl border'>
            <Icon className='size-5' aria-hidden='true' />
          </div>
          <h2 className='text-xl font-semibold tracking-tight'>
            {t(mode === 'image' ? 'Image Generation' : 'Video')}
          </h2>
          <p className='text-muted-foreground text-sm leading-6'>
            {t(
              'Test a model with a starter prompt, or write your own request below.'
            )}
          </p>
        </div>
      </div>
    )
  }

  return (
    <div className='h-full overflow-y-auto px-4 py-6 md:px-8'>
      <div className='mx-auto grid w-full max-w-4xl gap-4'>
        <div className='bg-muted/35 rounded-xl border p-4'>
          <p className='text-muted-foreground mb-1 text-xs font-medium'>
            {t('Prompt')}
          </p>
          <p className='text-sm leading-6 whitespace-pre-wrap'>
            {result.prompt}
          </p>
        </div>

        {result.status === 'generating' && (
          <div className='text-muted-foreground flex min-h-64 flex-col items-center justify-center gap-3 rounded-xl border border-dashed'>
            <LoaderCircle className='size-7 animate-spin' aria-hidden='true' />
            <p className='text-sm'>{t('Generating...')}</p>
            {mode === 'video' && typeof result.progress === 'number' && (
              <p className='text-xs'>
                {Math.max(0, Math.min(100, result.progress))}%
              </p>
            )}
          </div>
        )}

        {result.status === 'error' && (
          <Alert variant='destructive'>
            <AlertCircle />
            <AlertTitle>{t('Failed')}</AlertTitle>
            <AlertDescription>{result.errorMessage}</AlertDescription>
          </Alert>
        )}

        {result.status === 'success' && mode === 'image' && (
          <div className='grid gap-4 sm:grid-cols-2'>
            {result.images.map((src, index) => (
              <div
                className='bg-muted/20 overflow-hidden rounded-xl border'
                key={src}
              >
                <img
                  alt={`${t('Image')} ${index + 1}`}
                  className='max-h-[70svh] w-full object-contain'
                  src={src}
                />
              </div>
            ))}
          </div>
        )}

        {result.status === 'success' && mode === 'video' && result.videoUrl && (
          <div className='overflow-hidden rounded-xl border bg-black/90'>
            <video
              className='max-h-[70svh] w-full'
              controls
              playsInline
              src={result.videoUrl}
            />
          </div>
        )}
      </div>
    </div>
  )
}

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

import { StatusBadge } from '@/components/status-badge'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { formatTimestampToDate } from '@/lib/format'

interface RedemptionUserLinkProps {
  userId: number
  redeemedTime?: number
  onUserClick: (userId: number) => void
}

export function RedemptionUserLink({
  userId,
  redeemedTime = 0,
  onUserClick,
}: RedemptionUserLinkProps) {
  const { t } = useTranslation()

  if (userId === 0) {
    return <span className='text-muted-foreground text-sm'>-</span>
  }

  const userLabel = t('User {{id}}', { id: userId })

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger
          render={
            <button
              type='button'
              aria-label={`${t('User Information')}: ${userLabel}`}
              className='focus-visible:ring-ring cursor-pointer rounded-full text-left hover:brightness-95 focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none dark:hover:brightness-110'
              onClick={(event) => {
                event.stopPropagation()
                onUserClick(userId)
              }}
            >
              <StatusBadge
                label={userLabel}
                variant='neutral'
                copyable={false}
                className='pointer-events-none'
              />
            </button>
          }
        />
        <TooltipContent>
          <div className='space-y-1 text-xs'>
            <div>
              {t('User ID:')} {userId}
            </div>
            {redeemedTime > 0 && (
              <div>
                {t('Redeemed:')} {formatTimestampToDate(redeemedTime)}
              </div>
            )}
          </div>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}

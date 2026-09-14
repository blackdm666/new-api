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
import { UserRoundSearch } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { UserInfoDialog } from '@/components/user-info-dialog'

type AdminUserIdentityProps = {
  id: number
  username?: string
  remark?: string
  onClick?: () => void
}

export function AdminUserIdentity(props: AdminUserIdentityProps) {
  const { t } = useTranslation()
  const [userInfoOpen, setUserInfoOpen] = useState(false)
  const openUserInfo = props.onClick ?? (() => setUserInfoOpen(true))
  const content = (
    <>
      <div className='flex items-center gap-1.5 font-medium'>
        <span className='break-all group-hover:underline'>
          {props.username || '-'}
        </span>
        <UserRoundSearch
          className='text-muted-foreground size-3.5 shrink-0'
          aria-hidden='true'
        />
      </div>
      {props.remark?.trim() ? (
        <div
          className='text-muted-foreground mt-0.5 max-w-56 text-xs break-words'
          title={props.remark}
        >
          {t('Remark')}: {props.remark}
        </div>
      ) : null}
      <div className='text-muted-foreground text-xs'>UID {props.id}</div>
    </>
  )

  return (
    <>
      <button
        type='button'
        className='group focus-visible:ring-ring min-w-36 rounded-md text-left focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none'
        aria-label={`${t('User Information')}: ${props.username || `UID ${props.id}`}`}
        onClick={openUserInfo}
      >
        {content}
      </button>
      {props.onClick ? null : (
        <UserInfoDialog
          userId={props.id}
          open={userInfoOpen}
          onOpenChange={setUserInfoOpen}
        />
      )}
    </>
  )
}

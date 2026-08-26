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
import { Loader2 } from 'lucide-react'
import { type ReactNode, useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Label } from '@/components/ui/label'
import { api } from '@/lib/api'
import {
  formatCompactNumber,
  formatQuota,
  formatTimestampToDate,
} from '@/lib/format'

interface UserInfo {
  id: number
  username: string
  display_name?: string
  created_at: number
  last_login_at: number
  quota: number
  used_quota: number
  request_count: number
  group?: string
  inviter_id: number
  inviter_username?: string
  aff_count: number
  aff_quota?: number
  remark?: string
}

interface UserInfoDialogProps {
  userId: number | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

interface UserInfoResponse {
  success: boolean
  message?: string
  data?: UserInfo
}

function InfoItem(props: { label: string; value: string | number }) {
  return (
    <div className='min-w-0 space-y-1.5'>
      <Label className='text-muted-foreground text-xs'>{props.label}</Label>
      <div className='text-sm leading-relaxed font-semibold break-words whitespace-pre-wrap'>
        {props.value}
      </div>
    </div>
  )
}

async function getUserInfo(userId: number): Promise<UserInfoResponse> {
  const response = await api.get(`/api/user/${userId}`)
  return response.data
}

export function UserInfoDialog({
  userId,
  open,
  onOpenChange,
}: UserInfoDialogProps) {
  const { t } = useTranslation()
  const [userInfo, setUserInfo] = useState<UserInfo | null>(null)
  const [isLoading, setIsLoading] = useState(false)
  const [resolvedUserId, setResolvedUserId] = useState<number | null>(null)
  const requestSequence = useRef(0)

  const fetchUserInfo = useCallback(
    async (id: number) => {
      const requestId = ++requestSequence.current
      setIsLoading(true)
      setUserInfo(null)
      setResolvedUserId(null)
      try {
        const result = await getUserInfo(id)
        if (requestId !== requestSequence.current) return
        if (result.success) {
          setUserInfo(result.data || null)
        } else {
          toast.error(result.message || t('Failed to fetch user information'))
        }
      } catch (error) {
        if (requestId !== requestSequence.current) return
        // eslint-disable-next-line no-console
        console.error('Failed to fetch user info:', error)
        toast.error(t('Failed to fetch user information'))
      } finally {
        if (requestId === requestSequence.current) {
          setResolvedUserId(id)
          setIsLoading(false)
        }
      }
    },
    [t]
  )

  useEffect(() => {
    if (open && userId) {
      fetchUserInfo(userId)
    }
    return () => {
      requestSequence.current += 1
    }
  }, [open, userId, fetchUserInfo])

  const isWaitingForCurrentUser =
    open && userId !== null && userId > 0 && resolvedUserId !== userId

  let inviterValue = t('None')
  if (userInfo?.inviter_id) {
    inviterValue = userInfo.inviter_username
      ? `${userInfo.inviter_username} (ID: ${userInfo.inviter_id})`
      : `ID: ${userInfo.inviter_id}`
  }

  let content: ReactNode
  if (isLoading || isWaitingForCurrentUser) {
    content = (
      <div className='flex items-center justify-center py-8'>
        <Loader2 className='text-muted-foreground size-6 animate-spin' />
      </div>
    )
  } else if (userInfo) {
    content = (
      <div className='py-4'>
        <div className='grid grid-cols-1 gap-x-6 gap-y-5 sm:grid-cols-2'>
          <InfoItem label={t('Username')} value={userInfo.username} />
          <InfoItem
            label={t('Display Name')}
            value={userInfo.display_name || t('Not set')}
          />
          <InfoItem label={t('User ID')} value={userInfo.id} />
          <InfoItem
            label={t('Remark')}
            value={userInfo.remark?.trim() || t('Not set')}
          />
          <InfoItem
            label={t('Created At')}
            value={formatTimestampToDate(userInfo.created_at)}
          />
          <InfoItem
            label={t('Last Login')}
            value={
              userInfo.last_login_at
                ? formatTimestampToDate(userInfo.last_login_at)
                : t('Never logged in')
            }
          />
          <InfoItem label={t('Balance')} value={formatQuota(userInfo.quota)} />
          <InfoItem
            label={t('Used Quota')}
            value={formatQuota(userInfo.used_quota)}
          />
          <InfoItem
            label={t('Request Count')}
            value={formatCompactNumber(userInfo.request_count)}
          />
          <InfoItem
            label={t('User Group')}
            value={userInfo.group || t('Not set')}
          />
          <InfoItem label={t('Inviter')} value={inviterValue} />
          <InfoItem
            label={t('Invited User Count')}
            value={formatCompactNumber(userInfo.aff_count)}
          />
          {userInfo.aff_quota !== undefined && userInfo.aff_quota > 0 && (
            <InfoItem
              label={t('Invitation Quota')}
              value={formatQuota(userInfo.aff_quota)}
            />
          )}
        </div>
      </div>
    )
  } else {
    content = (
      <div className='text-muted-foreground py-8 text-center text-sm'>
        {t('No user information available')}
      </div>
    )
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('User Information')}
      description={t(
        'View detailed information about this user including balance, usage statistics, and invitation details.'
      )}
      contentClassName='sm:max-w-xl'
      contentHeight='auto'
      bodyClassName='space-y-4'
    >
      {content}
    </Dialog>
  )
}

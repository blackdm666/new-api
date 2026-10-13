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
import type { ReactNode } from 'react'
import type { Control } from 'react-hook-form'

import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'

import type { SettingsFormValues } from '../lib/forms'

type TextFieldName =
  | 'sub2api_base_url'
  | 'sub2api_admin_api_key'
  | 'sub2api_api_key_ids'
  | 'protected_channel_ids'
  | 'poll_interval_seconds'
  | 'lookback_minutes'
  | 'nosession_merge_seconds'
  | 'ban_group'
  | 'exempt_user_ids'

/** One labelled text/number input of the CyberGuard settings form. */
export function SettingsTextField(props: {
  control: Control<SettingsFormValues>
  name: TextFieldName
  label: ReactNode
  description?: ReactNode
  type?: 'text' | 'number' | 'password'
  placeholder?: string
  min?: number
  max?: number
}) {
  return (
    <FormField
      control={props.control}
      name={props.name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{props.label}</FormLabel>
          <FormControl>
            <Input
              {...field}
              type={props.type ?? 'text'}
              inputMode={props.type === 'number' ? 'numeric' : undefined}
              min={props.min}
              max={props.max}
              placeholder={props.placeholder}
              autoComplete={props.type === 'password' ? 'new-password' : 'off'}
            />
          </FormControl>
          {props.description && (
            <FormDescription>{props.description}</FormDescription>
          )}
          <FormMessage />
        </FormItem>
      )}
    />
  )
}

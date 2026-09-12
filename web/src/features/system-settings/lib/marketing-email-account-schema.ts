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
import type { TFunction } from 'i18next'
import * as z from 'zod'

export const createMarketingEmailAccountSchema = (t: TFunction) =>
  z
    .object({
      name: z.string().trim().min(1, t('Account name is required')),
      provider: z.literal('aliyun_eventbridge'),
      server: z
        .string()
        .trim()
        .min(1, t('SMTP host is required'))
        .refine(
          (value) =>
            value.toLowerCase() === 'smtpdm.aliyun.com' ||
            (/^smtpdm-[a-z0-9-]+\.aliyun\.com$/i.test(value) &&
              !value.includes('..')),
          t('Only Alibaba Cloud Direct Mail SMTP endpoints are supported')
        ),
      port: z
        .number()
        .int()
        .min(1, t('Port must be between 1 and 65535'))
        .max(65535, t('Port must be between 1 and 65535')),
      account: z.string().trim().min(1, t('Username is required')),
      from: z.string().trim().email(t('Enter a valid sender email')),
      token: z.string().trim(),
      ssl_enabled: z.boolean(),
      starttls_enabled: z.boolean(),
      insecure_skip_verify: z.boolean(),
      force_auth_login: z.boolean(),
      weight: z
        .number()
        .int()
        .min(1, t('Weight must be between 1 and 100'))
        .max(100, t('Weight must be between 1 and 100')),
      rate_limit_per_minute: z
        .number()
        .int()
        .min(1, t('RPM limit must be between 1 and 1000'))
        .max(1000, t('RPM limit must be between 1 and 1000')),
    })
    .superRefine((values, context) => {
      if (values.ssl_enabled && values.starttls_enabled) {
        context.addIssue({
          code: 'custom',
          path: ['starttls_enabled'],
          message: t('SSL/TLS and STARTTLS cannot both be enabled'),
        })
      }
    })

export type MarketingEmailAccountFormValues = z.infer<
  ReturnType<typeof createMarketingEmailAccountSchema>
>

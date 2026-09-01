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

const EMAIL_CATEGORY_LABELS: Record<string, string> = {
  email_verification: 'Registration and email verification',
  password_reset: 'Password reset',
  system_alert: 'System notification email',
  system_alert_user: 'System notification email',
  quota_warning_user: 'Quota warning',
  channel_status_admin: 'Channel status notification',
  inspection_alert_admin: 'Inspection alert',
  invoice_admin_email: 'Invoice administrator email',
  invoice_user_email: 'Invoice user email',
  affiliate_upgrade_admin: 'Promoter upgrade eligibility notification',
  affiliate_upgrade_user: 'Promoter tier upgrade notification',
  affiliate_commission_user: 'Commission review result notification',
  affiliate_payout_user: 'Commission payout status notification',
  marketing_custom: 'Custom campaign',
  marketing_registration_no_first_call:
    'Registration without first API request',
  marketing_single_topup: 'Single top-up win-back',
  marketing_single_topup_winback: 'Single top-up win-back',
  marketing_paid_low_balance: 'Paid user low balance',
  marketing_trial_low_balance: 'Trial balance almost depleted',
  marketing_inactive: 'Long-term inactive user',
  marketing_inactive_user: 'Long-term inactive user',
  marketing_affiliate_program_activation: 'Referral program activation',
  marketing_announcement: 'New announcement',
  email_preview: 'Test email',
}

export function emailCategoryLabel(
  category: string,
  t: (key: string) => string
) {
  const translationKey = EMAIL_CATEGORY_LABELS[category]
  return translationKey ? t(translationKey) : category
}

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
import { createInstance } from 'i18next'
import { describe, expect, test } from 'vitest'

import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'

describe('notification preview localization', () => {
  test('Chinese resources resolve notification labels and templates inside the translation namespace', async () => {
    const i18n = createInstance()
    await i18n.init({
      lng: 'zhCN',
      fallbackLng: 'en',
      resources: { en, zhCN: zh },
    })
    expect(i18n.t('User notifications')).toBe('用户通知')
    expect(i18n.t('Confirm simulated send')).toBe('确认模拟发送')
    expect(i18n.t('Policy warning example')).toContain('本邮件仅用于告知')
    await i18n.changeLanguage('en')
    expect(i18n.t('User notifications')).toBe('User notifications')
    expect(i18n.t('Policy warning example')).not.toBe('Policy warning example')
  })
})

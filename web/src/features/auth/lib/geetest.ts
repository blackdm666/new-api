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
import type { GeeTestValidation, SystemStatus } from '../types'

export type GeeTestScene = 'register' | 'login'

export interface GeeTestPublicConfig {
  enabled: boolean
  captchaId: string
}

export function getGeeTestPublicConfig(
  status: SystemStatus | null,
  scene: GeeTestScene
): GeeTestPublicConfig {
  if (scene === 'register') {
    const enabled =
      status?.geetest_register_check ??
      status?.data?.geetest_register_check ??
      false
    const captchaId =
      status?.geetest_register_captcha_id ??
      status?.data?.geetest_register_captcha_id ??
      ''
    return { enabled: Boolean(enabled && captchaId), captchaId }
  }

  const enabled =
    status?.geetest_login_check ?? status?.data?.geetest_login_check ?? false
  const captchaId =
    status?.geetest_login_captcha_id ??
    status?.data?.geetest_login_captcha_id ??
    ''
  return { enabled: Boolean(enabled && captchaId), captchaId }
}

export function getGeeTestQueryParams(validation?: GeeTestValidation) {
  if (!validation) return {}
  return {
    geetest_lot_number: validation.lot_number,
    geetest_captcha_output: validation.captcha_output,
    geetest_pass_token: validation.pass_token,
    geetest_gen_time: validation.gen_time,
  }
}

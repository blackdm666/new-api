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
// The recharge card shares the row with other cards on wide screens, so the
// column count follows the payment section's own width instead of the viewport:
// 2x2 when narrow, all four methods on one row once there is room.
export const paymentMethodSectionClassName =
  '@container/payment-methods space-y-2.5 sm:space-y-3'

export const paymentMethodGridClassName =
  'grid grid-cols-2 gap-1.5 sm:gap-3 @2xl/payment-methods:grid-cols-4'

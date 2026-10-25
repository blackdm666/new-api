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
import { CHANNEL_TYPE_GEMINI } from '../constants'

/**
 * Channel types whose adaptor can send non-streaming requests upstream as a
 * stream and rebuild one response. Mirrors the backend Gemini adaptor; the
 * per-channel toggle decides whether a channel actually does it.
 */
export function supportsForceUpstreamStream(channelType: number): boolean {
  return channelType === CHANNEL_TYPE_GEMINI
}

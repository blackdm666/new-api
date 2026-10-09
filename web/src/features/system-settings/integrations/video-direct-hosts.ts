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
export const VIDEO_DIRECT_HOSTS_OPTION_KEY = 'TaskVideoDirectHosts'
export const MAX_VIDEO_DIRECT_HOSTS = 256
export const MAX_VIDEO_DIRECT_HOSTS_LENGTH = 8192

const HOST_LABEL = /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/

// Mirrors ValidateTaskVideoDirectHosts; the server remains authoritative.
export function parseVideoDirectHosts(value: string): string[] {
  return value
    .split(/[,;\s]+/)
    .map((entry) => entry.trim().toLowerCase().replace(/\.$/, ''))
    .filter(Boolean)
}

export function isValidVideoDirectHost(entry: string): boolean {
  const host = entry.startsWith('*.') ? entry.slice(2) : entry
  if (!host || host.includes('*') || host.length > 253) return false
  if (/^\d{1,3}(?:\.\d{1,3}){3}$/.test(host)) return false
  const labels = host.split('.')
  if (labels.length < 2) return false
  if (!labels.every((label) => HOST_LABEL.test(label))) return false
  return !/^\d+$/.test(labels.at(-1) ?? '')
}

export function formatVideoDirectHosts(value: string): string {
  return parseVideoDirectHosts(value).join('\n')
}

export function serializeVideoDirectHosts(value: string): string {
  return parseVideoDirectHosts(value).join(',')
}

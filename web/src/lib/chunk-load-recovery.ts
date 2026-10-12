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

const RELOAD_AT_STORAGE_KEY = 'chunk-load-reload-at'
// 冷却期内不再自动刷新，避免分包确实缺失时陷入刷新循环
const RELOAD_COOLDOWN_MS = 30_000

// rspack: "Loading chunk 123 failed." / "Loading CSS chunk 123 failed."
// 浏览器原生动态 import（Chrome / Firefox / Safari）
const CHUNK_LOAD_ERROR_MESSAGE =
  /^(Loading (CSS )?chunk \S+ failed|Failed to fetch dynamically imported module|error loading dynamically imported module|Importing a module script failed)/

/**
 * 发版后，打开中的旧页面仍引用旧哈希的路由分包，服务器上已不存在，懒加载会失败。
 * TanStack Router 只识别原生 ESM 的报错，不认 rspack 的 ChunkLoadError。
 */
export function isChunkLoadError(error: unknown): boolean {
  if (!(error instanceof Error)) return false
  if (error.name === 'ChunkLoadError') return true
  return CHUNK_LOAD_ERROR_MESSAGE.test(error.message)
}

/**
 * 分包加载失败时整页刷新一次，重新获取新版本入口。
 * 返回 true 表示已触发刷新；冷却期内或无法记录刷新时间时返回 false，由调用方展示错误页。
 */
export function reloadAfterChunkLoadError(error: unknown): boolean {
  if (!isChunkLoadError(error)) return false

  const now = Date.now()
  try {
    const lastReloadAt = Number(sessionStorage.getItem(RELOAD_AT_STORAGE_KEY))
    if (lastReloadAt && now - lastReloadAt < RELOAD_COOLDOWN_MS) return false
    sessionStorage.setItem(RELOAD_AT_STORAGE_KEY, String(now))
  } catch {
    return false
  }

  window.location.reload()
  return true
}

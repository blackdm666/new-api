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
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  lazyRouteComponent,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { GeneralError } from '../general-error'

const reload = vi.fn()

function rspackChunkLoadError(): Error {
  const error = new Error(
    'Loading chunk 4821 failed.\n(missing: https://88api.ai/static/js/async/4821.0a1b2c3d.js)'
  )
  error.name = 'ChunkLoadError'
  return error
}

function renderRouteThatFailsWith(error: Error) {
  const rootRoute = createRootRoute({
    component: Outlet,
    errorComponent: GeneralError,
  })
  const pageRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/',
    component: lazyRouteComponent(() => Promise.reject(error)),
  })
  const router = createRouter({
    routeTree: rootRoute.addChildren([pageRoute]),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(<RouterProvider router={router} />)
}

beforeEach(() => {
  sessionStorage.clear()
  vi.spyOn(window, 'location', 'get').mockReturnValue({
    ...window.location,
    reload,
  })
})

afterEach(() => {
  cleanup()
  sessionStorage.clear()
})

it('reloads once without showing the error page when a stale route chunk fails to load', async () => {
  renderRouteThatFailsWith(rspackChunkLoadError())

  await waitFor(() => expect(reload).toHaveBeenCalledTimes(1))
  expect(screen.queryByText(/Oops! Something went wrong/)).not.toBeInTheDocument()
})

it('shows the error page instead of reloading again when the chunk still fails right after a reload', async () => {
  sessionStorage.setItem('chunk-load-reload-at', String(Date.now() - 5_000))

  renderRouteThatFailsWith(rspackChunkLoadError())

  expect(
    await screen.findByText(/Oops! Something went wrong/)
  ).toBeInTheDocument()
  expect(reload).not.toHaveBeenCalled()
})

it('reloads again once the previous automatic reload is outside the cooldown', async () => {
  sessionStorage.setItem('chunk-load-reload-at', String(Date.now() - 60_000))

  renderRouteThatFailsWith(rspackChunkLoadError())

  await waitFor(() => expect(reload).toHaveBeenCalledTimes(1))
})

it('shows the error page without reloading when a route fails for another reason', async () => {
  renderRouteThatFailsWith(
    new TypeError("Cannot read properties of undefined (reading 'map')")
  )

  expect(
    await screen.findByText(/Oops! Something went wrong/)
  ).toBeInTheDocument()
  expect(reload).not.toHaveBeenCalled()
})

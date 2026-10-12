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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, test } from 'vitest'

import { TopNavDropdown } from '../components/top-nav-dropdown'
import type { TopNavLink } from '../types'

const businessLink: TopNavLink = {
  title: 'Business',
  href: '/business/',
  external: true,
  sameTab: true,
  trackId: 'business',
  overviewTitle: 'Business overview',
  items: [
    {
      title: 'Enterprise',
      description: 'Contracts and bank transfer',
      href: '/business/enterprise/',
      trackId: 'business_enterprise',
    },
    {
      title: 'Partner Program',
      description: 'Referral commission',
      href: '/business/partner/',
      trackId: 'business_partner',
    },
  ],
}

// jsdom cannot navigate: record whether the component cancelled each click,
// then stop the navigation after React has handled the event.
let clicksCancelledByComponent: boolean[] = []
const stopNavigation = (event: MouseEvent) => {
  clicksCancelledByComponent.push(event.defaultPrevented)
  event.preventDefault()
}

function renderDropdown() {
  render(<TopNavDropdown link={businessLink} navLocation='console_header' />)
  return screen.getByRole('link', { name: 'Business' })
}

describe('TopNavDropdown', () => {
  beforeEach(() => {
    window.dataLayer = []
    clicksCancelledByComponent = []
    window.addEventListener('click', stopNavigation)
  })

  afterEach(() => {
    delete window.dataLayer
    window.removeEventListener('click', stopNavigation)
  })

  test('renders the trigger as a same-tab link to the overview page', () => {
    const trigger = renderDropdown()

    expect(trigger).toHaveAttribute('href', '/business/')
    expect(trigger).not.toHaveAttribute('target')
    expect(trigger).toHaveAttribute('aria-expanded', 'false')
  })

  test('opens with ArrowDown and lists same-tab links with descriptions', async () => {
    const trigger = renderDropdown()

    fireEvent.keyDown(trigger, { key: 'ArrowDown' })

    const enterprise = await screen.findByRole('link', { name: /Enterprise/ })
    expect(trigger).toHaveAttribute('aria-expanded', 'true')
    expect(enterprise).toHaveAttribute('href', '/business/enterprise/')
    expect(enterprise).not.toHaveAttribute('target')
    expect(enterprise).toHaveTextContent('Contracts and bank transfer')
    expect(
      screen.getByRole('link', { name: /Partner Program/ })
    ).toHaveAttribute('href', '/business/partner/')
    expect(
      screen.getByRole('link', { name: /Business overview/ })
    ).toHaveAttribute('href', '/business/')
  })

  test('closes with Escape', async () => {
    const trigger = renderDropdown()
    fireEvent.keyDown(trigger, { key: 'ArrowDown' })
    const enterprise = await screen.findByRole('link', { name: /Enterprise/ })

    fireEvent.keyDown(enterprise, { key: 'Escape' })

    await waitFor(() =>
      expect(trigger).toHaveAttribute('aria-expanded', 'false')
    )
  })

  test('reports a trigger click as console_header nav_click', () => {
    const trigger = renderDropdown()

    fireEvent.click(trigger)

    expect(window.dataLayer).toEqual([
      {
        event: 'nav_click',
        nav_item: 'business',
        nav_location: 'console_header',
      },
    ])
  })

  test('reports a dropdown link click as dropdown nav_click', async () => {
    const trigger = renderDropdown()
    fireEvent.keyDown(trigger, { key: 'ArrowDown' })

    fireEvent.click(
      await screen.findByRole('link', { name: /Partner Program/ })
    )

    expect(window.dataLayer).toEqual([
      {
        event: 'nav_click',
        nav_item: 'business_partner',
        nav_location: 'dropdown',
      },
    ])
  })

  test('follows the trigger link without GTM on the page', () => {
    delete window.dataLayer
    const trigger = renderDropdown()

    fireEvent.click(trigger)

    expect(clicksCancelledByComponent).toEqual([false])
  })

  test('opens on the first touch tap and follows the link on the second', async () => {
    const trigger = renderDropdown()

    fireEvent.pointerDown(trigger, { pointerType: 'touch' })
    fireEvent.click(trigger, { detail: 1 })

    await waitFor(() =>
      expect(trigger).toHaveAttribute('aria-expanded', 'true')
    )
    expect(clicksCancelledByComponent).toEqual([true])
    expect(window.dataLayer).toEqual([])

    fireEvent.pointerDown(trigger, { pointerType: 'touch' })
    fireEvent.click(trigger, { detail: 1 })

    expect(clicksCancelledByComponent).toEqual([true, false])
    expect(window.dataLayer).toEqual([
      {
        event: 'nav_click',
        nav_item: 'business',
        nav_location: 'console_header',
      },
    ])
  })
})

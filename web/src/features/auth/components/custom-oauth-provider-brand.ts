/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
export type CustomOAuthProviderIdentity = {
  name?: string
  slug?: string
  icon?: string
}

export type CustomOAuthBrand = 'google' | 'microsoft' | null

export function resolveCustomOAuthBrand(
  provider: CustomOAuthProviderIdentity
): CustomOAuthBrand {
  const identity = [provider.icon, provider.slug, provider.name]
    .filter(Boolean)
    .join(' ')
    .toLowerCase()

  if (identity.includes('google')) return 'google'
  if (identity.includes('microsoft')) return 'microsoft'
  return null
}
